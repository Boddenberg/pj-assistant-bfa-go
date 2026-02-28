package service

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================
// Register — POST /v1/auth/register
// ============================================================

func (s *AuthService) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.RegisterResponse, error) {
	ctx, span := authTracer.Start(ctx, "AuthService.Register")
	defer span.End()

	// Normalize: strip masks so storage is always digits-only
	req.CNPJ = normalizeDoc(req.CNPJ)
	req.RepresentanteCPF = normalizeDoc(req.RepresentanteCPF)

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
