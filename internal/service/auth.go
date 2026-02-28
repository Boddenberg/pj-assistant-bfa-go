// Package service — auth.go defines the AuthService struct, constructor,
// constants and shared helpers. The actual methods are split across:
//
//   - auth_registration.go — Register
//   - auth_login.go        — Login, devLoginFallback
//   - auth_tokens.go       — Refresh, Logout, ValidateAccessToken, JWT helpers
//   - auth_password.go     — PasswordResetRequest, PasswordResetConfirm, ChangePassword
//   - auth_profile.go      — UpdateProfile, UpdateRepresentative
package service

import (
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
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
	devAuth    bool
	logger     *zap.Logger
}

// NewAuthService creates a new auth service.
func NewAuthService(store port.AuthStore, jwtSecret string, accessTTL, refreshTTL time.Duration, devAuth bool, logger *zap.Logger) *AuthService {
	return &AuthService{
		store:      store,
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		devAuth:    devAuth,
		logger:     logger,
	}
}

// normalizeDoc removes all non-digit characters from a document number (CPF/CNPJ).
func normalizeDoc(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
