package port

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// CreditCardStore handles credit card data operations.
type CreditCardStore interface {
	CreateCreditCard(ctx context.Context, customerID string, req *domain.CreditCardRequest) (*domain.CreditCard, error)
	ListCreditCards(ctx context.Context, customerID string) ([]domain.CreditCard, error)
	GetCreditCard(ctx context.Context, customerID, cardID string) (*domain.CreditCard, error)
	UpdateCreditCardStatus(ctx context.Context, cardID, status string) error
	UpdateCreditCardLimit(ctx context.Context, customerID string, newLimit float64) error
	UpdateCreditCardUsedLimit(ctx context.Context, cardID string, usedLimit, availableLimit float64) error
	UpdateCreditCardPixCreditUsed(ctx context.Context, cardID string, pixCreditUsed float64) error
}

// CreditCardTransactionStore handles credit card transaction data operations.
type CreditCardTransactionStore interface {
	ListCreditCardTransactions(ctx context.Context, customerID, cardID string, page, pageSize int) ([]domain.CreditCardTransaction, error)
	InsertCreditCardTransaction(ctx context.Context, data map[string]any) error
}

// CreditCardInvoiceStore handles credit card invoice data operations.
type CreditCardInvoiceStore interface {
	ListCreditCardInvoices(ctx context.Context, customerID, cardID string) ([]domain.CreditCardInvoice, error)
	GetCreditCardInvoice(ctx context.Context, customerID, cardID, invoiceID string) (*domain.CreditCardInvoice, error)
	GetCreditCardInvoiceByMonth(ctx context.Context, customerID, cardID, month string) (*domain.CreditCardInvoice, error)
	CreateCreditCardInvoice(ctx context.Context, invoice map[string]any) (*domain.CreditCardInvoice, error)
	UpdateCreditCardInvoiceStatus(ctx context.Context, invoiceID, status string) error
	UpdateCreditCardInvoiceTotals(ctx context.Context, invoiceID string, totalAmount, minimumPayment float64) error
}
