package chatv2

import "go.uber.org/zap"

// AccountRepository é a interface de persistência.
// Stub em memória para o MVP.
type AccountRepository interface {
	CNPJExists(cnpj string) bool
	FinalizeAccount(data map[string]string) error
}

// --- Stub in-memory ---

type InMemoryAccountRepository struct {
	cnpjs  map[string]bool
	logger *zap.Logger
}

func NewInMemoryAccountRepository(logger *zap.Logger) *InMemoryAccountRepository {
	return &InMemoryAccountRepository{
		cnpjs:  make(map[string]bool),
		logger: logger,
	}
}

func (r *InMemoryAccountRepository) CNPJExists(cnpj string) bool {
	return r.cnpjs[cnpj]
}

func (r *InMemoryAccountRepository) FinalizeAccount(data map[string]string) error {
	cnpj := data["cnpj"]
	r.cnpjs[cnpj] = true
	r.logger.Info("conta PJ criada com sucesso (stub)",
		zap.String("cnpj", cnpj),
		zap.String("razao_social", data["razaoSocial"]),
		zap.String("email", data["email"]),
	)
	return nil
}
