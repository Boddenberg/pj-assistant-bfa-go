package service

import (
	"context"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ============================================================
// PIX Keys — lookup, registration, deletion
// ============================================================

func (s *BankingService) ListPixKeys(ctx context.Context, customerID string) ([]domain.PixKey, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListPixKeys")
	defer span.End()

	return s.store.ListPixKeys(ctx, customerID)
}

func (s *BankingService) LookupPixKey(ctx context.Context, keyType, keyValue string) (*domain.PixKey, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.LookupPixKey")
	defer span.End()

	if keyValue == "" {
		return nil, &domain.ErrValidation{Field: "key", Message: "key is required"}
	}

	// Auto-detect keyType from value format if not provided
	if keyType == "" {
		keyType = detectPixKeyType(keyValue)
	}

	// If we have a keyType, search with it; otherwise search by value only
	if keyType != "" {
		return s.store.LookupPixKey(ctx, keyType, keyValue)
	}
	return s.store.LookupPixKeyByValue(ctx, keyValue)
}

// detectPixKeyType infers the pix key type from the value format.
func detectPixKeyType(value string) string {
	// Strip non-digit chars for numeric checks
	digits := ""
	hasCNPJFormatting := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
		if r == '.' || r == '/' {
			hasCNPJFormatting = true
		}
	}

	// Email — check first since it's unambiguous
	if strings.Contains(value, "@") {
		return "email"
	}
	// UUID-like → random
	if len(value) == 36 && strings.Count(value, "-") == 4 {
		return "random"
	}
	// CNPJ: 14 digits, or 11-14 digits with CNPJ formatting (dots/slashes)
	if len(digits) == 14 {
		return "cnpj"
	}
	if hasCNPJFormatting && len(digits) >= 11 && len(digits) <= 14 {
		return "cnpj"
	}
	// CPF: 11 digits (not starting with +)
	if len(digits) == 11 && !strings.HasPrefix(value, "+") {
		return "cpf"
	}
	// Phone: starts with + or has 10-13 digits (only if no CNPJ formatting)
	if strings.HasPrefix(value, "+") {
		return "phone"
	}
	if len(digits) >= 10 && len(digits) <= 13 && !hasCNPJFormatting {
		return "phone"
	}
	// Could not determine
	return ""
}

// GetCustomerName resolves a customer ID to a human-readable name.
func (s *BankingService) GetCustomerName(ctx context.Context, customerID string) (string, error) {
	return s.store.GetCustomerName(ctx, customerID)
}

// GetCustomerLookupData returns full profile + account data for pix lookup responses.
func (s *BankingService) GetCustomerLookupData(ctx context.Context, customerID string) (name, document, bank, branch, account string, err error) {
	return s.store.GetCustomerLookupData(ctx, customerID)
}

// DeletePixKey removes a Pix key for the given customer.
func (s *BankingService) DeletePixKey(ctx context.Context, customerID, keyID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeletePixKey")
	defer span.End()

	if customerID == "" || keyID == "" {
		return &domain.ErrValidation{Field: "keyId", Message: "required"}
	}

	err := s.store.DeletePixKey(ctx, customerID, keyID)
	if err != nil {
		s.logger.Error("failed to delete pix key",
			zap.String("customer_id", customerID),
			zap.String("key_id", keyID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("pix key deleted",
		zap.String("customer_id", customerID),
		zap.String("key_id", keyID),
	)
	return nil
}

// DeletePixKeyByValue removes a Pix key by its type and value.
func (s *BankingService) DeletePixKeyByValue(ctx context.Context, customerID, keyType, keyValue string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeletePixKeyByValue")
	defer span.End()

	// Lookup the key to get its ID
	key, err := s.store.LookupPixKey(ctx, keyType, keyValue)
	if err != nil {
		return err
	}
	// Verify it belongs to the customer
	if key.CustomerID != customerID {
		return &domain.ErrNotFound{Resource: "pix_key", ID: keyValue}
	}

	return s.store.DeletePixKey(ctx, customerID, key.ID)
}

// RegisterPixKey creates a new Pix key for the given customer.
func (s *BankingService) RegisterPixKey(ctx context.Context, req *domain.PixKeyRegisterRequest) (*domain.PixKeyRegisterResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.RegisterPixKey")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", req.CustomerID))

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}

	validTypes := map[string]bool{"cnpj": true, "email": true, "phone": true, "random": true}
	if !validTypes[req.KeyType] {
		return nil, &domain.ErrValidation{Field: "keyType", Message: "deve ser cnpj, email, phone ou random"}
	}

	// Get primary account for account_id
	account, err := s.store.GetPrimaryAccount(ctx, req.CustomerID)
	if err != nil {
		return nil, err
	}

	keyValue := req.KeyValue
	if req.KeyType == "random" {
		keyValue = uuid.New().String()
	} else if keyValue == "" {
		return nil, &domain.ErrValidation{Field: "keyValue", Message: "required for non-random key type"}
	}

	key := &domain.PixKey{
		ID:         uuid.New().String(),
		AccountID:  account.ID,
		CustomerID: req.CustomerID,
		KeyType:    req.KeyType,
		KeyValue:   keyValue,
		Status:     "active",
		CreatedAt:  time.Now(),
	}

	created, err := s.store.CreatePixKey(ctx, key)
	if err != nil {
		s.logger.Error("failed to register pix key",
			zap.String("customer_id", req.CustomerID),
			zap.String("key_type", req.KeyType),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("pix key registered",
		zap.String("customer_id", req.CustomerID),
		zap.String("key_type", req.KeyType),
		zap.String("key_id", created.ID),
	)

	return &domain.PixKeyRegisterResponse{
		KeyID:     created.ID,
		KeyType:   created.KeyType,
		KeyValue:  created.KeyValue,
		Key:       created.KeyValue,
		Status:    created.Status,
		CreatedAt: created.CreatedAt.Format(time.RFC3339),
	}, nil
}

// GetCreditLimit returns the total credit limit for a customer's cards.
func (s *BankingService) GetCreditLimit(ctx context.Context, customerID string) (float64, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCreditLimit")
	defer span.End()

	cards, err := s.store.ListCreditCards(ctx, customerID)
	if err != nil {
		return 0, err
	}
	if len(cards) == 0 {
		return 0, &domain.ErrNotFound{Resource: "credit_card", ID: customerID}
	}

	// Return the highest credit limit among all cards
	var maxLimit float64
	for _, c := range cards {
		if c.CreditLimit > maxLimit {
			maxLimit = c.CreditLimit
		}
	}
	return maxLimit, nil
}
