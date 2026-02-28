package service

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

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
