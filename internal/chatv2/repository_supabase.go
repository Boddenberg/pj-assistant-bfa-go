package chatv2

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"go.uber.org/zap"
)

// SupabaseAccountRepository implementa AccountRepository usando Supabase.
type SupabaseAccountRepository struct {
	sb     *supabase.Client
	logger *zap.Logger
}

func NewSupabaseAccountRepository(sb *supabase.Client, logger *zap.Logger) *SupabaseAccountRepository {
	return &SupabaseAccountRepository{sb: sb, logger: logger}
}

func (r *SupabaseAccountRepository) CNPJExists(ctx context.Context, cnpj string) bool {
	// Verificar em customer_profiles (contas já criadas)
	existing, _ := r.sb.GetCustomerByDocument(ctx, cnpj)
	if existing != nil {
		return true
	}
	// Verificar em onboarding_sessions (já completou onboarding)
	exists, _ := r.sb.CNPJExistsInOnboarding(ctx, cnpj)
	return exists
}

func (r *SupabaseAccountRepository) SaveField(ctx context.Context, sessionID, step, value string) error {
	return r.sb.UpsertOnboardingField(ctx, sessionID, step, value)
}

func (r *SupabaseAccountRepository) FinalizeAccount(ctx context.Context, sessionID string, data map[string]string) (*AccountData, error) {
	// Montar RegisterRequest com os dados do onboarding
	req := &domain.RegisterRequest{
		CNPJ:                   data["cnpj"],
		RazaoSocial:            data["razaoSocial"],
		NomeFantasia:           data["nomeFantasia"],
		Email:                  data["email"],
		RepresentanteName:      data["representanteName"],
		RepresentanteCPF:       data["representanteCpf"],
		RepresentantePhone:     data["representantePhone"],
		RepresentanteBirthDate: data["representanteBirthDate"],
		Password:               data["password"],
	}

	// Usar o mesmo fluxo de criação de conta que já existe
	resp, err := r.sb.CreateCustomerWithAccount(ctx, req, data["password"])
	if err != nil {
		return nil, fmt.Errorf("create account via supabase: %w", err)
	}

	// Marcar sessão de onboarding como completa
	if err := r.sb.CompleteOnboardingSession(ctx, sessionID, resp.CustomerID); err != nil {
		r.logger.Warn("failed to mark onboarding as completed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	r.logger.Info("🎉 conta PJ criada via onboarding",
		zap.String("session_id", sessionID),
		zap.String("customer_id", resp.CustomerID),
		zap.String("agencia", resp.Agencia),
		zap.String("conta", resp.Conta),
	)

	return &AccountData{
		CustomerID: resp.CustomerID,
		Agencia:    resp.Agencia,
		Conta:      resp.Conta,
	}, nil
}
