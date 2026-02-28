package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================
// Login — POST /v1/auth/login
// ============================================================

func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest) (*domain.LoginResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.Login")
	defer span.End()

	// Normalize: strip mask so lookup works regardless of format sent
	req.CPF = normalizeDoc(req.CPF)
	span.SetAttributes(attribute.String("cpf", req.CPF))

	// Find customer by representante CPF
	profile, err := s.store.GetCustomerByCPF(ctx, req.CPF)
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
			zap.String("cpf", req.CPF),
		)
		return nil, &domain.ErrAccountBlocked{Status: "blocked"}
	}

	// Get credentials
	cred, err := s.store.GetCredentials(ctx, profile.CustomerID)
	if err != nil {
		var notFound *domain.ErrNotFound
		if errors.As(err, &notFound) {
			// No bcrypt credentials — try dev_logins fallback if enabled
			if s.devAuth {
				return s.devLoginFallback(ctx, profile, req.Password)
			}
			// Corrupted registration: profile exists but no credentials were saved.
			// Treat as invalid credentials to avoid leaking internal state.
			s.logger.Warn("login: credentials not found for existing profile (corrupted registration)",
				zap.String("customer_id", profile.CustomerID),
				zap.String("cpf", req.CPF),
			)
			return nil, &domain.ErrUnauthorized{Message: "Credenciais inválidas"}
		}
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

// devLoginFallback is called when DEV_AUTH=true and auth_credentials is missing.
// It checks the dev_logins table (plain-text passwords) and, if matched,
// issues a real JWT so the rest of the flow works normally.
func (s *AuthService) devLoginFallback(ctx context.Context, profile *domain.CustomerProfile, password string) (*domain.LoginResponse, error) {
	s.logger.Warn("DEV_AUTH: attempting dev_logins fallback",
		zap.String("customer_id", profile.CustomerID),
	)

	devProfile, err := s.store.DevLoginLookup(ctx, profile.RepresentanteCPF, password)
	if err != nil {
		return nil, fmt.Errorf("dev login lookup: %w", err)
	}
	if devProfile == nil {
		return nil, &domain.ErrUnauthorized{Message: "Credenciais inválidas"}
	}

	accessToken, err := s.signAccessToken(devProfile.CustomerID, devProfile.Document)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}
	refreshToken, _, err := s.generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	s.logger.Info("DEV_AUTH: login successful via dev_logins",
		zap.String("customer_id", devProfile.CustomerID),
	)

	return &domain.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		CustomerID:   devProfile.CustomerID,
		CustomerName: devProfile.Name,
		CompanyName:  devProfile.CompanyName,
	}, nil
}
