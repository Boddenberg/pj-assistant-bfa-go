package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

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
// Internal helpers
// ============================================================

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
