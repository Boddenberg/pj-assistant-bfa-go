// Package service — AuthService handles authentication, registration,
// JWT token management, password reset and profile updates.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var authTracer = otel.Tracer("service/auth")

const (
	maxFailedAttempts = 5
	lockDuration      = 30 * time.Minute
	bcryptCost        = 12
)

// AuthService orchestrates authentication flows.
type AuthService struct {
	store      port.AuthStore
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	logger     *zap.Logger
}

// NewAuthService creates a new auth service.
func NewAuthService(store port.AuthStore, jwtSecret string, accessTTL, refreshTTL time.Duration, logger *zap.Logger) *AuthService {
	return &AuthService{
		store:      store,
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		logger:     logger,
	}
}

// ============================================================
// Register — POST /v1/auth/register
// ============================================================

func (s *AuthService) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.RegisterResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.Register")
	defer span.End()

	// Check if CNPJ already registered
	existing, err := s.store.GetCustomerByDocument(ctx, req.CNPJ)
	if err != nil {
		return nil, fmt.Errorf("check existing customer: %w", err)
	}
	if existing != nil {
		return nil, &domain.ErrConflict{Message: "CNPJ já cadastrado"}
	}

	// Validate 6-digit password
	if len(req.Password) != 6 {
		return nil, &domain.ErrValidation{Field: "password", Message: "Senha deve ter 6 dígitos"}
	}
	for _, c := range req.Password {
		if c < '0' || c > '9' {
			return nil, &domain.ErrValidation{Field: "password", Message: "Senha deve conter apenas dígitos"}
		}
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create customer + account + credentials
	resp, err := s.store.CreateCustomerWithAccount(ctx, req, string(hash))
	if err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}

	s.logger.Info("customer registered",
		zap.String("customer_id", resp.CustomerID),
		zap.String("cnpj", req.CNPJ),
	)

	return resp, nil
}

// ============================================================
// Login — POST /v1/auth/login
// ============================================================

func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest) (*domain.LoginResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.Login")
	defer span.End()
	span.SetAttributes(attribute.String("document", req.Document))

	// Find customer by document + agencia + conta
	profile, err := s.store.GetCustomerByBankDetails(ctx, req.Document, req.Agencia, req.Conta)
	if err != nil {
		return nil, fmt.Errorf("get customer: %w", err)
	}
	if profile == nil {
		return nil, &domain.ErrUnauthorized{Message: "Credenciais inválidas"}
	}

	// Check account status
	if profile.AccountStatus == "blocked" {
		s.logger.Warn("login: account blocked",
			zap.String("customer_id", profile.CustomerID),
			zap.String("document", req.Document),
		)
		return nil, &domain.ErrAccountBlocked{Status: "blocked"}
	}

	// Get credentials
	cred, err := s.store.GetCredentials(ctx, profile.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("get credentials: %w", err)
	}

	// Check if account is locked
	if cred.LockedUntil != nil && cred.LockedUntil.After(time.Now()) {
		remaining := time.Until(*cred.LockedUntil).Minutes()
		s.logger.Warn("login: account temporarily locked",
			zap.String("customer_id", profile.CustomerID),
			zap.Float64("remaining_minutes", remaining),
		)
		return nil, &domain.ErrUnauthorized{
			Message: fmt.Sprintf("Conta temporariamente bloqueada. Tente novamente em %.0f minutos", remaining),
		}
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.Password)); err != nil {
		// Increment failed attempts
		newAttempts := cred.FailedAttempts + 1
		updates := map[string]any{"failed_attempts": newAttempts}
		if newAttempts >= maxFailedAttempts {
			lockedUntil := time.Now().Add(lockDuration)
			updates["locked_until"] = lockedUntil.Format(time.RFC3339)
			s.logger.Warn("login: account locked after max attempts",
				zap.String("customer_id", profile.CustomerID),
				zap.Int("attempts", newAttempts),
				zap.Duration("lock_duration", lockDuration),
			)
		} else {
			s.logger.Warn("login: failed password attempt",
				zap.String("customer_id", profile.CustomerID),
				zap.Int("attempts", newAttempts),
				zap.Int("max", maxFailedAttempts),
			)
		}
		_ = s.store.UpdateCredentials(ctx, profile.CustomerID, updates)

		remaining := maxFailedAttempts - newAttempts
		if remaining <= 0 {
			return nil, &domain.ErrUnauthorized{
				Message: fmt.Sprintf("Conta bloqueada por %d minutos após %d tentativas", int(lockDuration.Minutes()), maxFailedAttempts),
			}
		}
		return nil, &domain.ErrUnauthorized{
			Message: fmt.Sprintf("Credenciais inválidas. %d tentativa(s) restante(s)", remaining),
		}
	}

	// Reset failed attempts on successful login
	_ = s.store.UpdateCredentials(ctx, profile.CustomerID, map[string]any{
		"failed_attempts": 0,
		"locked_until":    nil,
		"last_login_at":   time.Now().Format(time.RFC3339),
	})

	// Generate tokens
	accessToken, err := s.signAccessToken(profile.CustomerID, profile.Document)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshToken, refreshHash, err := s.generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store refresh token hash
	if err := s.store.StoreRefreshToken(ctx, profile.CustomerID, refreshHash, time.Now().Add(s.refreshTTL)); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	s.logger.Info("customer logged in", zap.String("customer_id", profile.CustomerID))

	return &domain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		CustomerID:   profile.CustomerID,
		CustomerName: profile.Name,
		CompanyName:  profile.CompanyName,
	}, nil
}

// ============================================================
// Refresh — POST /v1/auth/refresh
// ============================================================

func (s *AuthService) Refresh(ctx context.Context, req *domain.RefreshRequest) (*domain.LoginResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.Refresh")
	defer span.End()

	// Hash the incoming refresh token to look up
	tokenHash := hashToken(req.RefreshToken)

	stored, err := s.store.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	if stored == nil {
		return nil, &domain.ErrUnauthorized{Message: "Token de atualização inválido"}
	}

	// Check expiry
	if stored.ExpiresAt.Before(time.Now()) {
		s.logger.Warn("refresh: expired token used",
			zap.String("customer_id", stored.CustomerID),
		)
		_ = s.store.RevokeRefreshToken(ctx, tokenHash)
		return nil, &domain.ErrUnauthorized{Message: "Token de atualização expirado"}
	}

	// Revoke old token (rotation)
	_ = s.store.RevokeRefreshToken(ctx, tokenHash)

	// Get customer profile by ID from the stored token
	customerID := stored.CustomerID
	profile, err := s.store.GetCustomerByID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("get customer profile: %w", err)
	}

	document := ""
	customerName := ""
	companyName := ""
	if profile != nil {
		document = profile.Document
		customerName = profile.Name
		companyName = profile.CompanyName
	}

	// Generate new tokens
	accessToken, err := s.signAccessToken(customerID, document)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	newRefreshToken, newRefreshHash, err := s.generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.store.StoreRefreshToken(ctx, customerID, newRefreshHash, time.Now().Add(s.refreshTTL)); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &domain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		CustomerID:   customerID,
		CustomerName: customerName,
		CompanyName:  companyName,
	}, nil
}

// ============================================================
// Logout — POST /v1/auth/logout
// ============================================================

func (s *AuthService) Logout(ctx context.Context, customerID string) error {
	ctx, span := authTracer.Start(ctx, "AuthService.Logout")
	defer span.End()

	if err := s.store.RevokeAllRefreshTokens(ctx, customerID); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}

	s.logger.Info("customer logged out", zap.String("customer_id", customerID))
	return nil
}

// ============================================================
// PasswordResetRequest — POST /v1/auth/password/reset-request
// ============================================================

func (s *AuthService) PasswordResetRequest(ctx context.Context, req *domain.PasswordResetRequestBody) (*domain.PasswordResetRequestResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.PasswordResetRequest")
	defer span.End()

	profile, err := s.store.GetCustomerByBankDetails(ctx, req.Document, req.Agencia, req.Conta)
	if err != nil {
		return nil, fmt.Errorf("get customer: %w", err)
	}
	if profile == nil {
		// Return success anyway (don't leak whether account exists)
		return &domain.PasswordResetRequestResponse{
			Message:     "Se os dados estiverem corretos, enviaremos o código de verificação",
			MaskedEmail: "***@***.com",
			ExpiresIn:   600,
		}, nil
	}

	// Generate 6-digit code
	code := generateVerificationCode()
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := s.store.StoreResetCode(ctx, profile.CustomerID, code, expiresAt); err != nil {
		return nil, fmt.Errorf("store reset code: %w", err)
	}

	// In production, send email/SMS here
	s.logger.Info("password reset code generated",
		zap.String("customer_id", profile.CustomerID),
		zap.String("code", code), // ONLY in dev — remove in production
	)

	return &domain.PasswordResetRequestResponse{
		Message:     "Código de verificação enviado",
		MaskedEmail: maskEmail(profile.Email),
		ExpiresIn:   600,
	}, nil
}

// ============================================================
// PasswordResetConfirm — POST /v1/auth/password/reset-confirm
// ============================================================

func (s *AuthService) PasswordResetConfirm(ctx context.Context, req *domain.PasswordResetConfirmRequest) error {
	ctx, span := authTracer.Start(ctx, "AuthService.PasswordResetConfirm")
	defer span.End()

	profile, err := s.store.GetCustomerByBankDetails(ctx, req.Document, req.Agencia, req.Conta)
	if err != nil {
		return fmt.Errorf("get customer: %w", err)
	}
	if profile == nil {
		return &domain.ErrUnauthorized{Message: "Credenciais inválidas"}
	}

	// Validate code
	resetCode, err := s.store.GetValidResetCode(ctx, profile.CustomerID, req.VerificationCode)
	if err != nil {
		return fmt.Errorf("get reset code: %w", err)
	}
	if resetCode == nil {
		return &domain.ErrInvalidCode{}
	}

	// Validate new password
	if len(req.NewPassword) != 6 {
		return &domain.ErrValidation{Field: "newPassword", Message: "Senha deve ter 6 dígitos"}
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Update credentials
	if err := s.store.UpdateCredentials(ctx, profile.CustomerID, map[string]any{
		"password_hash":       string(hash),
		"failed_attempts":     0,
		"locked_until":        nil,
		"password_changed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("update credentials: %w", err)
	}

	// Mark code as used
	_ = s.store.MarkResetCodeUsed(ctx, resetCode.ID)

	// Revoke all refresh tokens (force re-login)
	_ = s.store.RevokeAllRefreshTokens(ctx, profile.CustomerID)

	s.logger.Info("password reset completed", zap.String("customer_id", profile.CustomerID))
	return nil
}

// ============================================================
// ChangePassword — PUT /v1/auth/password
// ============================================================

func (s *AuthService) ChangePassword(ctx context.Context, customerID string, req *domain.ChangePasswordRequest) error {
	ctx, span := authTracer.Start(ctx, "AuthService.ChangePassword")
	defer span.End()

	cred, err := s.store.GetCredentials(ctx, customerID)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		s.logger.Warn("password change: wrong current password",
			zap.String("customer_id", customerID),
		)
		return &domain.ErrUnauthorized{Message: "Senha atual incorreta"}
	}

	// Validate new password
	if len(req.NewPassword) != 6 {
		return &domain.ErrValidation{Field: "newPassword", Message: "Senha deve ter 6 dígitos"}
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdateCredentials(ctx, customerID, map[string]any{
		"password_hash":       string(hash),
		"password_changed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("update credentials: %w", err)
	}

	// Revoke all refresh tokens (force re-login on other devices)
	_ = s.store.RevokeAllRefreshTokens(ctx, customerID)

	s.logger.Info("password changed", zap.String("customer_id", customerID))
	return nil
}

// ============================================================
// UpdateProfile — PUT /v1/customers/{id}/profile
// ============================================================

func (s *AuthService) UpdateProfile(ctx context.Context, customerID string, req *domain.UpdateProfileRequest) (*domain.UpdateProfileResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.UpdateProfile")
	defer span.End()

	updates := map[string]any{}
	if req.NomeFantasia != "" {
		updates["company_name"] = req.NomeFantasia
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.RepresentantePhone != "" {
		updates["representante_phone"] = req.RepresentantePhone
	}

	if len(updates) == 0 {
		return nil, &domain.ErrValidation{Field: "body", Message: "Nenhum campo para atualizar"}
	}

	profile, err := s.store.UpdateCustomerProfile(ctx, customerID, updates)
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}

	return &domain.UpdateProfileResponse{
		ID:                profile.CustomerID,
		Name:              profile.Name,
		Document:          profile.Document,
		CompanyName:       profile.CompanyName,
		Segment:           profile.Segment,
		AccountStatus:     profile.AccountStatus,
		RelationshipSince: profile.RelationshipSince,
		CreditScore:       profile.CreditScore,
	}, nil
}

// ============================================================
// UpdateRepresentative — PUT /v1/customers/{id}/representative
// ============================================================

func (s *AuthService) UpdateRepresentative(ctx context.Context, customerID string, req *domain.UpdateRepresentativeRequest) (*domain.UpdateRepresentativeResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.UpdateRepresentative")
	defer span.End()

	updates := map[string]any{}
	if req.RepresentanteName != "" {
		updates["representante_name"] = req.RepresentanteName
	}
	if req.RepresentantePhone != "" {
		updates["representante_phone"] = req.RepresentantePhone
	}

	if len(updates) == 0 {
		return nil, &domain.ErrValidation{Field: "body", Message: "Nenhum campo para atualizar"}
	}

	profile, err := s.store.UpdateRepresentative(ctx, customerID, updates)
	if err != nil {
		return nil, fmt.Errorf("update representative: %w", err)
	}

	return &domain.UpdateRepresentativeResponse{
		Message:                "Dados do representante atualizados com sucesso",
		RepresentanteName:      profile.RepresentanteName,
		RepresentanteCPF:       profile.RepresentanteCPF,
		RepresentantePhone:     profile.RepresentantePhone,
		RepresentanteBirthDate: profile.RepresentanteBirthDate,
	}, nil
}

// ============================================================
// ValidateToken — used by middleware
// ============================================================

// JWTClaims represents the custom claims in access tokens.
type JWTClaims struct {
	Sub  string `json:"sub"`
	CNPJ string `json:"cnpj"`
	Type string `json:"type"`
	jwt.RegisteredClaims
}

func (s *AuthService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, &domain.ErrUnauthorized{Message: "Token inválido ou expirado"}
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, &domain.ErrUnauthorized{Message: "Token inválido"}
	}

	if claims.Type != "access" {
		return nil, &domain.ErrUnauthorized{Message: "Tipo de token inválido"}
	}

	return claims, nil
}

// ============================================================
// Internal helpers
// ============================================================

func (s *AuthService) signAccessToken(customerID, cnpj string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Sub:  customerID,
		CNPJ: cnpj,
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			Issuer:    "bfa-api",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) generateRefreshToken() (raw string, hashed string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	hashed = hashToken(raw)
	return raw, hashed, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateVerificationCode() string {
	code := ""
	for i := 0; i < 6; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += fmt.Sprintf("%d", n.Int64())
	}
	return code
}

func maskEmail(email string) string {
	if email == "" {
		return "***@***.com"
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***@***.com"
	}
	local := parts[0]
	domainParts := parts[1]

	masked := string(local[0])
	if len(local) > 1 {
		masked += strings.Repeat("*", len(local)-2)
		masked += string(local[len(local)-1])
	} else {
		masked += "***"
	}
	return masked + "@" + domainParts
}
