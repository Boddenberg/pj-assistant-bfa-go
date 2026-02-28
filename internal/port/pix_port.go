package port

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// PixKeyStore handles PIX key data operations.
type PixKeyStore interface {
	ListPixKeys(ctx context.Context, customerID string) ([]domain.PixKey, error)
	LookupPixKey(ctx context.Context, keyType, keyValue string) (*domain.PixKey, error)
	LookupPixKeyByValue(ctx context.Context, keyValue string) (*domain.PixKey, error)
	CreatePixKey(ctx context.Context, key *domain.PixKey) (*domain.PixKey, error)
	DeletePixKey(ctx context.Context, customerID, keyID string) error
}

// PixTransferStore handles PIX transfer data operations.
type PixTransferStore interface {
	CreatePixTransfer(ctx context.Context, customerID string, req *domain.PixTransferRequest) (*domain.PixTransfer, error)
	ListPixTransfers(ctx context.Context, customerID string, page, pageSize int) ([]domain.PixTransfer, error)
	GetPixTransfer(ctx context.Context, customerID, transferID string) (*domain.PixTransfer, error)
	UpdatePixTransferStatus(ctx context.Context, transferID, status string) error
}

// PixReceiptStore handles PIX receipt data operations.
type PixReceiptStore interface {
	SavePixReceipt(ctx context.Context, receipt *domain.PixReceipt) (*domain.PixReceipt, error)
	GetPixReceipt(ctx context.Context, receiptID string) (*domain.PixReceipt, error)
	GetPixReceiptByTransferID(ctx context.Context, transferID string) (*domain.PixReceipt, error)
	ListPixReceipts(ctx context.Context, customerID string) ([]domain.PixReceipt, error)
}

// CustomerLookupStore resolves customer identity for PIX operations.
type CustomerLookupStore interface {
	GetCustomerName(ctx context.Context, customerID string) (string, error)
	GetCustomerLookupData(ctx context.Context, customerID string) (name, document, bank, branch, account string, err error)
}

// ScheduledTransferStore handles scheduled transfer data operations.
type ScheduledTransferStore interface {
	CreateScheduledTransfer(ctx context.Context, customerID string, req *domain.ScheduledTransferRequest) (*domain.ScheduledTransfer, error)
	ListScheduledTransfers(ctx context.Context, customerID string) ([]domain.ScheduledTransfer, error)
	GetScheduledTransfer(ctx context.Context, customerID, transferID string) (*domain.ScheduledTransfer, error)
	UpdateScheduledTransferStatus(ctx context.Context, transferID, status string) error
}
