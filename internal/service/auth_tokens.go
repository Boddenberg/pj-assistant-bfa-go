package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

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
// Internal JWT helpers
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
