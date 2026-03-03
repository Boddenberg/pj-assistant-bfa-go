package chat

import (
	"context"

	"go.uber.org/zap"
)

// AccountRepository é a interface de persistência do onboarding.
type AccountRepository interface {
	// CNPJExists verifica se o CNPJ já está cadastrado.
	CNPJExists(ctx context.Context, cnpj string) bool
	// CPFExists verifica se o CPF já está cadastrado.
	CPFExists(ctx context.Context, cpf string) bool
	// SaveField salva um campo validado no banco (por session_id).
	SaveField(ctx context.Context, sessionID, step, value string) error
	// FinalizeAccount cria a conta real e retorna os dados da conta.
	FinalizeAccount(ctx context.Context, sessionID string, data map[string]string) (*AccountData, error)
	// LoadSession carrega os dados já preenchidos de uma sessão do banco.
	// Retorna map[step]value. Se a sessão não existir, retorna nil.
	LoadSession(ctx context.Context, sessionID string) (map[string]string, error)
	// DeleteSession remove os dados temporários da sessão do banco.
	DeleteSession(ctx context.Context, sessionID string) error
}

// AccountData são os dados da conta criada, retornados ao frontend.
type AccountData struct {
	CustomerID string `json:"customerId"`
	Agencia    string `json:"agencia"`
	Conta      string `json:"conta"`
}

// --- Stub in-memory (para testes e dev sem Supabase) ---

type InMemoryAccountRepository struct {
	cnpjs  map[string]bool
	cpfs   map[string]bool
	logger *zap.Logger
}

func NewInMemoryAccountRepository(logger *zap.Logger) *InMemoryAccountRepository {
	return &InMemoryAccountRepository{
		cnpjs:  make(map[string]bool),
		cpfs:   make(map[string]bool),
		logger: logger,
	}
}

func (r *InMemoryAccountRepository) CNPJExists(_ context.Context, cnpj string) bool {
	return r.cnpjs[cnpj]
}

func (r *InMemoryAccountRepository) CPFExists(_ context.Context, cpf string) bool {
	return r.cpfs[cpf]
}

func (r *InMemoryAccountRepository) SaveField(_ context.Context, sessionID, step, value string) error {
	r.logger.Info("stub: field saved",
		zap.String("session_id", sessionID),
		zap.String("step", step),
	)
	return nil
}

func (r *InMemoryAccountRepository) LoadSession(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil // stub: sem persistência, nunca retoma
}

func (r *InMemoryAccountRepository) DeleteSession(_ context.Context, _ string) error {
	return nil // stub: nada para deletar
}

func (r *InMemoryAccountRepository) FinalizeAccount(_ context.Context, sessionID string, data map[string]string) (*AccountData, error) {
	cnpj := data["cnpj"]
	r.cnpjs[cnpj] = true
	r.logger.Info("stub: conta PJ criada",
		zap.String("session_id", sessionID),
		zap.String("cnpj", cnpj),
		zap.String("razao_social", data["razaoSocial"]),
	)
	return &AccountData{
		CustomerID: "stub-" + sessionID,
		Agencia:    "0001",
		Conta:      "123456-7",
	}, nil
}
