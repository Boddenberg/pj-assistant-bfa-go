package port

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// BillingStore handles bill payment and debit purchase data operations.
type BillingStore interface {
	CreateBillPayment(ctx context.Context, customerID string, req *domain.BillPaymentRequest, validation *domain.BarcodeValidationResponse) (*domain.BillPayment, error)
	ListBillPayments(ctx context.Context, customerID string, page, pageSize int) ([]domain.BillPayment, error)
	GetBillPayment(ctx context.Context, customerID, billID string) (*domain.BillPayment, error)
	UpdateBillPaymentStatus(ctx context.Context, billID, status string) error
	ListDebitPurchases(ctx context.Context, customerID string, page, pageSize int) ([]domain.DebitPurchase, error)
	CreateDebitPurchase(ctx context.Context, customerID string, req *domain.DebitPurchaseRequest) (*domain.DebitPurchase, error)
}
