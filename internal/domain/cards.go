package domain

import "time"

// ============================================================
// Credit Card PJ
// ============================================================

// CreditCardRequest is the payload to request a new PJ credit card.
type CreditCardRequest struct {
	AccountID      string  `json:"account_id"`
	CardBrand      string  `json:"card_brand,omitempty"` // Visa, Mastercard, Elo
	CardType       string  `json:"card_type,omitempty"`  // corporate, virtual, additional
	BillingDay     int     `json:"billing_day,omitempty"`
	DueDay         int     `json:"due_day,omitempty"`
	RequestedLimit float64 `json:"requested_limit,omitempty"`
}

// CreditCard represents a PJ credit card.
type CreditCard struct {
	ID               string     `json:"id"`
	CustomerID       string     `json:"customer_id"`
	AccountID        string     `json:"account_id"`
	CardNumberLast4  string     `json:"card_number_last4"`
	CardHolderName   string     `json:"card_holder_name"`
	CardBrand        string     `json:"card_brand"`
	CardType         string     `json:"card_type"`
	CreditLimit      float64    `json:"credit_limit"`
	AvailableLimit   float64    `json:"available_limit"`
	UsedLimit        float64    `json:"used_limit"`
	BillingDay       int        `json:"billing_day"`
	DueDay           int        `json:"due_day"`
	Status           string     `json:"status"`
	PixCreditEnabled bool       `json:"pix_credit_enabled"`
	PixCreditLimit   float64    `json:"pix_credit_limit"`
	PixCreditUsed    float64    `json:"pix_credit_used"`
	IsContactless    bool       `json:"is_contactless_enabled"`
	IsInternational  bool       `json:"is_international_enabled"`
	IsOnline         bool       `json:"is_online_enabled"`
	DailyLimit       float64    `json:"daily_limit"`
	SingleTxLimit    float64    `json:"single_transaction_limit"`
	IssuedAt         *time.Time `json:"issued_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CreditCardTransaction represents a purchase or charge on a credit card.
type CreditCardTransaction struct {
	ID                 string    `json:"id"`
	CardID             string    `json:"card_id"`
	CustomerID         string    `json:"customer_id"`
	TransactionDate    time.Time `json:"transaction_date"`
	Amount             float64   `json:"amount"`
	OriginalAmount     *float64  `json:"original_amount,omitempty"`
	InstallmentAmount  *float64  `json:"installment_amount,omitempty"`
	MerchantName       string    `json:"merchant_name"`
	Category           string    `json:"category"`
	Installments       int       `json:"installments"`
	CurrentInstallment int       `json:"current_installment"`
	TransactionType    string    `json:"transaction_type"`
	Status             string    `json:"status"`
	Description        string    `json:"description,omitempty"`
	IsInternational    bool      `json:"is_international"`
}

// CreditCardInvoice represents a monthly credit card bill.
type CreditCardInvoice struct {
	ID             string    `json:"id"`
	CardID         string    `json:"card_id"`
	CustomerID     string    `json:"customer_id"`
	ReferenceMonth string    `json:"reference_month"` // "2026-02"
	OpenDate       string    `json:"open_date"`
	CloseDate      string    `json:"close_date"`
	DueDate        string    `json:"due_date"`
	TotalAmount    float64   `json:"total_amount"`
	MinimumPayment float64   `json:"minimum_payment"`
	InterestAmount float64   `json:"interest_amount"`
	Status         string    `json:"status"`
	PaidAmount     *float64  `json:"paid_amount,omitempty"`
	Barcode        string    `json:"barcode,omitempty"`
	DigitableLine  string    `json:"digitable_line,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ============================================================
// Credit Card API Response types (matches frontend spec)
// ============================================================

// CreditCardAPIResponse is returned by GET /v1/customers/{id}/cards.
type CreditCardAPIResponse struct {
	ID             string  `json:"id"`
	LastFourDigits string  `json:"lastFourDigits"`
	Brand          string  `json:"brand"`
	Status         string  `json:"status"`
	Limit          float64 `json:"limit"`
	AvailableLimit float64 `json:"availableLimit"`
	UsedLimit      float64 `json:"usedLimit"`
	DueDay         int     `json:"dueDay"`
	ClosingDay     int     `json:"closingDay"`
	AnnualFee      float64 `json:"annualFee"`
	IsVirtual      bool    `json:"isVirtual"`
	CreatedAt      string  `json:"createdAt"`
}

// CreditCardRequestBody is the body for POST /v1/cards/request.
type CreditCardRequestBody struct {
	CustomerID     string  `json:"customerId"`
	PreferredBrand string  `json:"preferredBrand,omitempty"`
	RequestedLimit float64 `json:"requestedLimit"`
	DueDay         int     `json:"dueDay,omitempty"`
	VirtualCard    bool    `json:"virtualCard"`
}

// CreditCardRequestResponse is returned by POST /v1/cards/request.
type CreditCardRequestResponse struct {
	RequestID             string                 `json:"requestId"`
	Status                string                 `json:"status"` // approved, denied, under_review
	Card                  *CreditCardAPIResponse `json:"card,omitempty"`
	Message               string                 `json:"message"`
	ApprovedLimit         float64                `json:"approvedLimit,omitempty"`
	EstimatedDeliveryDays int                    `json:"estimatedDeliveryDays,omitempty"`
}

// CreditCardInvoiceAPIResponse is returned by GET /v1/cards/{id}/invoices/{month}.
type CreditCardInvoiceAPIResponse struct {
	ID             string                       `json:"id"`
	CardID         string                       `json:"cardId"`
	ReferenceMonth string                       `json:"referenceMonth"`
	TotalAmount    float64                      `json:"totalAmount"`
	MinimumPayment float64                      `json:"minimumPayment"`
	DueDate        string                       `json:"dueDate"`
	Status         string                       `json:"status"`
	Transactions   []InvoiceTransactionResponse `json:"transactions"`
}

// InvoiceTransactionResponse is a transaction within an invoice.
type InvoiceTransactionResponse struct {
	ID                string   `json:"id"`
	Date              string   `json:"date"`
	Description       string   `json:"description"`
	Amount            float64  `json:"amount"`
	OriginalAmount    *float64 `json:"originalAmount,omitempty"`
	FeeAmount         *float64 `json:"feeAmount,omitempty"`
	TotalWithFees     *float64 `json:"totalWithFees,omitempty"`
	InstallmentAmount *float64 `json:"installmentAmount,omitempty"`
	Installment       string   `json:"installment,omitempty"`
	Category          string   `json:"category"`
}

// InvoicePayRequest is the body for paying a credit card invoice.
type InvoicePayRequest struct {
	Amount      float64 `json:"amount"`
	PaymentType string  `json:"paymentType"` // total, minimum, custom
}

// InvoicePayResponse is returned after paying a credit card invoice.
type InvoicePayResponse struct {
	PaymentID        string  `json:"paymentId"`
	Status           string  `json:"status"`
	Amount           float64 `json:"amount"`
	PaidAt           string  `json:"paidAt"`
	NewInvoiceStatus string  `json:"newInvoiceStatus"`
}
