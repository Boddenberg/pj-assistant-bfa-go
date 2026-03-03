package chatv2

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
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

func (r *SupabaseAccountRepository) CPFExists(ctx context.Context, cpf string) bool {
	existing, _ := r.sb.GetCustomerByCPF(ctx, cpf)
	return existing != nil
}

func (r *SupabaseAccountRepository) SaveField(ctx context.Context, sessionID, step, value string) error {
	return r.sb.UpsertOnboardingField(ctx, sessionID, step, value)
}

func (r *SupabaseAccountRepository) LoadSession(ctx context.Context, sessionID string) (map[string]string, error) {
	row, err := r.sb.GetOnboardingSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	// Se já completou, não retomar
	if row.Status == "completed" {
		return nil, nil
	}

	// Converter row para map[step]value (só campos preenchidos)
	data := make(map[string]string)
	if row.CNPJ != "" {
		data["cnpj"] = row.CNPJ
	}
	if row.RazaoSocial != "" {
		data["razaoSocial"] = row.RazaoSocial
	}
	if row.NomeFantasia != "" {
		data["nomeFantasia"] = row.NomeFantasia
	}
	if row.Email != "" {
		data["email"] = row.Email
	}
	if row.RepresentanteName != "" {
		data["representanteName"] = row.RepresentanteName
	}
	if row.RepresentanteCPF != "" {
		data["representanteCpf"] = row.RepresentanteCPF
	}
	if row.RepresentantePhone != "" {
		data["representantePhone"] = row.RepresentantePhone
	}
	if row.RepresentanteBirthDate != "" {
		data["representanteBirthDate"] = row.RepresentanteBirthDate
	}
	if row.PasswordHash != "" {
		data["password"] = row.PasswordHash
	}

	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

func (r *SupabaseAccountRepository) DeleteSession(ctx context.Context, sessionID string) error {
	r.logger.Info("🗑️  deletando sessão temporária do banco",
		zap.String("session_id", sessionID),
	)
	return r.sb.DeleteOnboardingSession(ctx, sessionID)
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

	// Hash da senha com bcrypt (mesmo custo usado pelo auth service)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(data["password"]), 12)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Usar o mesmo fluxo de criação de conta que já existe
	resp, err := r.sb.CreateCustomerWithAccount(ctx, req, string(passwordHash))
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
