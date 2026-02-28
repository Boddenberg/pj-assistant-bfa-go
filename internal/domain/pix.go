package domain

import "time"

// ============================================================
// PIX Keys
// ============================================================

// PixKey represents a registered PIX key.
type PixKey struct {
	ID         string    `json:"id"`
	AccountID  string    `json:"account_id"`
	CustomerID string    `json:"customer_id"`
	KeyType    string    `json:"key_type"` // cpf, cnpj, email, phone, random
	KeyValue   string    `json:"key_value"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ============================================================
// PIX Transfer
// ============================================================

// PixTransferRequest is the payload to initiate a PIX transfer.
type PixTransferRequest struct {
	IdempotencyKey         string  `json:"idempotency_key"`
	SourceAccountID        string  `json:"source_account_id"`
	DestinationKeyType     string  `json:"destination_key_type"`
	DestinationKeyValue    string  `json:"destination_key_value"`
	DestinationName        string  `json:"destination_name,omitempty"`
	DestinationDocument    string  `json:"destination_document,omitempty"`
	Amount                 float64 `json:"amount"`
	Description            string  `json:"description,omitempty"`
	FundedBy               string  `json:"funded_by,omitempty"` // "balance" or "credit_card"
	CreditCardID           string  `json:"credit_card_id,omitempty"`
	CreditCardInstallments int     `json:"credit_card_installments,omitempty"`
	FeeRate                float64 `json:"fee_rate,omitempty"`        // e.g. 0.02 for 2% per installment
	TotalWithFees          float64 `json:"total_with_fees,omitempty"` // amount * (1 + feeRate*(installments-1))
	ScheduledFor           string  `json:"scheduled_for,omitempty"`   // RFC3339 or empty for immediate
}

// PixTransfer represents a PIX transfer record.
type PixTransfer struct {
	ID                     string     `json:"id"`
	IdempotencyKey         string     `json:"idempotency_key"`
	SourceAccountID        string     `json:"source_account_id"`
	SourceCustomerID       string     `json:"source_customer_id"`
	DestinationKeyType     string     `json:"destination_key_type"`
	DestinationKeyValue    string     `json:"destination_key_value"`
	DestinationName        string     `json:"destination_name,omitempty"`
	DestinationDocument    string     `json:"destination_document,omitempty"`
	Amount                 float64    `json:"amount"`
	Description            string     `json:"description,omitempty"`
	Status                 string     `json:"status"`
	FailureReason          string     `json:"failure_reason,omitempty"`
	EndToEndID             string     `json:"end_to_end_id,omitempty"`
	FundedBy               string     `json:"funded_by"`
	CreditCardID           string     `json:"credit_card_id,omitempty"`
	CreditCardInstallments int        `json:"credit_card_installments,omitempty"`
	ScheduledFor           *time.Time `json:"scheduled_for,omitempty"`
	ExecutedAt             *time.Time `json:"executed_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	ReceiptID              string     `json:"receipt_id,omitempty"` // set in memory after receipt creation
}

// PixReceipt represents a Pix transfer receipt (comprovante).
type PixReceipt struct {
	ID                string  `json:"id"`
	TransferID        string  `json:"transfer_id"`
	CustomerID        string  `json:"customer_id"`
	Direction         string  `json:"direction"` // "sent" or "received"
	Amount            float64 `json:"amount"`
	OriginalAmount    float64 `json:"original_amount,omitempty"`
	FeeAmount         float64 `json:"fee_amount,omitempty"`
	TotalAmount       float64 `json:"total_amount,omitempty"`
	Description       string  `json:"description,omitempty"`
	EndToEndID        string  `json:"end_to_end_id"`
	FundedBy          string  `json:"funded_by"`
	Installments      int     `json:"installments,omitempty"`
	SenderName        string  `json:"sender_name"`
	SenderDocument    string  `json:"sender_document"`
	SenderBank        string  `json:"sender_bank"`
	SenderBranch      string  `json:"sender_branch"`
	SenderAccount     string  `json:"sender_account"`
	RecipientName     string  `json:"recipient_name"`
	RecipientDocument string  `json:"recipient_document"`
	RecipientBank     string  `json:"recipient_bank"`
	RecipientBranch   string  `json:"recipient_branch"`
	RecipientAccount  string  `json:"recipient_account"`
	RecipientKeyType  string  `json:"recipient_key_type"`
	RecipientKeyValue string  `json:"recipient_key_value"`
	TransactionID     string  `json:"transaction_id,omitempty"`
	Status            string  `json:"status"`
	ExecutedAt        string  `json:"executed_at"`
	CreatedAt         string  `json:"created_at"`
}

// ============================================================
// Scheduled Transfers
// ============================================================

// ScheduledTransferRequest is the payload to create a scheduled transfer.
type ScheduledTransferRequest struct {
	IdempotencyKey      string  `json:"idempotency_key"`
	SourceAccountID     string  `json:"source_account_id"`
	TransferType        string  `json:"transfer_type"` // pix, ted, doc, internal
	DestinationBankCode string  `json:"destination_bank_code"`
	DestinationBranch   string  `json:"destination_branch"`
	DestinationAccount  string  `json:"destination_account"`
	DestinationAcctType string  `json:"destination_account_type"`
	DestinationName     string  `json:"destination_name"`
	DestinationDocument string  `json:"destination_document"`
	Amount              float64 `json:"amount"`
	Description         string  `json:"description,omitempty"`
	ScheduleType        string  `json:"schedule_type"`  // once, daily, weekly, biweekly, monthly
	ScheduledDate       string  `json:"scheduled_date"` // YYYY-MM-DD
	RecurrenceEndDate   string  `json:"recurrence_end_date,omitempty"`
	MaxRecurrences      *int    `json:"max_recurrences,omitempty"`
}

// ScheduledTransfer represents a scheduled transfer record.
type ScheduledTransfer struct {
	ID                  string     `json:"id"`
	IdempotencyKey      string     `json:"idempotency_key"`
	SourceAccountID     string     `json:"source_account_id"`
	SourceCustomerID    string     `json:"source_customer_id"`
	TransferType        string     `json:"transfer_type"`
	DestinationBankCode string     `json:"destination_bank_code"`
	DestinationBranch   string     `json:"destination_branch"`
	DestinationAccount  string     `json:"destination_account"`
	DestinationAcctType string     `json:"destination_account_type"`
	DestinationName     string     `json:"destination_name"`
	DestinationDocument string     `json:"destination_document"`
	Amount              float64    `json:"amount"`
	Description         string     `json:"description,omitempty"`
	ScheduleType        string     `json:"schedule_type"`
	ScheduledDate       string     `json:"scheduled_date"`
	NextExecutionDate   string     `json:"next_execution_date,omitempty"`
	RecurrenceCount     int        `json:"recurrence_count"`
	MaxRecurrences      *int       `json:"max_recurrences,omitempty"`
	Status              string     `json:"status"`
	FailureReason       string     `json:"failure_reason,omitempty"`
	LastExecutedAt      *time.Time `json:"last_executed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ============================================================
// PIX API Response types (matches frontend spec)
// ============================================================

// PixRecipient represents the destination of a PIX transfer.
type PixRecipient struct {
	Name     string      `json:"name"`
	Document string      `json:"document"`
	Bank     string      `json:"bank"`
	Branch   string      `json:"branch"`
	Account  string      `json:"account"`
	PixKey   *PixKeyInfo `json:"pixKey,omitempty"`
}

// PixKeyInfo is a key type + value pair.
type PixKeyInfo struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// PixKeyLookupResponse is returned by GET /v1/pix/keys/lookup.
type PixKeyLookupResponse struct {
	Recipient *PixRecipient `json:"recipient"`
	KeyType   string        `json:"keyType"`
}

// PixTransferResponse is returned by POST /v1/pix/transfer.
type PixTransferResponse struct {
	TransactionID string        `json:"transactionId"`
	Status        string        `json:"status"`
	Amount        float64       `json:"amount"`
	NewBalance    float64       `json:"newBalance,omitempty"`
	Recipient     *PixRecipient `json:"recipient"`
	Timestamp     string        `json:"timestamp"`
	E2EID         string        `json:"e2eId"`
	ReceiptID     string        `json:"receiptId,omitempty"`
}

// PixScheduleRequest is the body for POST /v1/pix/schedule.
type PixScheduleRequest struct {
	CustomerID       string              `json:"customerId"`
	RecipientKey     string              `json:"recipientKey"`
	RecipientKeyType string              `json:"recipientKeyType"`
	Amount           float64             `json:"amount"`
	ScheduledDate    string              `json:"scheduledDate"`
	Description      string              `json:"description,omitempty"`
	Recurrence       *ScheduleRecurrence `json:"recurrence,omitempty"`
}

// ScheduleRecurrence specifies recurring schedule details.
type ScheduleRecurrence struct {
	Type    string `json:"type"` // weekly, monthly
	EndDate string `json:"endDate,omitempty"`
}

// PixScheduleResponse is returned by schedule endpoints.
type PixScheduleResponse struct {
	ScheduleID    string              `json:"scheduleId"`
	Status        string              `json:"status"`
	Amount        float64             `json:"amount"`
	ScheduledDate string              `json:"scheduledDate"`
	Recipient     *PixRecipient       `json:"recipient"`
	Recurrence    *ScheduleRecurrence `json:"recurrence,omitempty"`
}

// PixCreditCardRequest is the body for POST /v1/pix/credit-card.
type PixCreditCardRequest struct {
	CustomerID       string  `json:"customerId"`
	CreditCardID     string  `json:"creditCardId"`
	RecipientKey     string  `json:"recipientKey"`
	RecipientKeyType string  `json:"recipientKeyType"`
	Amount           float64 `json:"amount"`
	Installments     int     `json:"installments"`
	Description      string  `json:"description,omitempty"`
}

// PixCreditCardResponse is returned by POST /v1/pix/credit-card.
type PixCreditCardResponse struct {
	TransactionID    string        `json:"transactionId"`
	Status           string        `json:"status"`
	Amount           float64       `json:"amount"`
	OriginalAmount   float64       `json:"originalAmount"`
	FeeAmount        float64       `json:"feeAmount"`
	TotalWithFees    float64       `json:"totalWithFees"`
	Installments     int           `json:"installments"`
	InstallmentValue float64       `json:"installmentValue"`
	Recipient        *PixRecipient `json:"recipient"`
	Timestamp        string        `json:"timestamp"`
	ReceiptID        string        `json:"receiptId,omitempty"`
}

// PixReceiptResponse is the formatted receipt returned to the frontend.
type PixReceiptResponse struct {
	ID             string           `json:"id"`
	TransferID     string           `json:"transferId"`
	Direction      string           `json:"direction"`
	Amount         float64          `json:"amount"`
	OriginalAmount float64          `json:"originalAmount,omitempty"`
	FeeAmount      float64          `json:"feeAmount,omitempty"`
	TotalAmount    float64          `json:"totalAmount,omitempty"`
	Description    string           `json:"description,omitempty"`
	E2EID          string           `json:"e2eId"`
	FundedBy       string           `json:"fundedBy"`
	Installments   int              `json:"installments,omitempty"`
	Sender         *PixReceiptParty `json:"sender"`
	Recipient      *PixReceiptParty `json:"recipient"`
	PixKey         *PixKeyInfo      `json:"pixKey,omitempty"`
	Status         string           `json:"status"`
	ExecutedAt     string           `json:"executedAt"`
	CreatedAt      string           `json:"createdAt"`
}

// PixReceiptParty represents a sender or recipient in a receipt.
type PixReceiptParty struct {
	Name     string `json:"name"`
	Document string `json:"document"`
	Bank     string `json:"bank"`
	Branch   string `json:"branch,omitempty"`
	Account  string `json:"account,omitempty"`
}

// PixKeyRegisterRequest is the body for POST /v1/pix/keys/register.
type PixKeyRegisterRequest struct {
	CustomerID string `json:"customerId"`
	KeyType    string `json:"keyType"`  // cnpj, email, phone, random
	KeyValue   string `json:"keyValue"` // empty for random
}

// PixKeyRegisterResponse is returned by POST /v1/pix/keys/register.
type PixKeyRegisterResponse struct {
	KeyID     string `json:"keyId"`
	KeyType   string `json:"keyType"`
	KeyValue  string `json:"keyValue"`
	Key       string `json:"key"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}
