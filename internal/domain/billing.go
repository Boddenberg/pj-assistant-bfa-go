package domain

import "time"

// ============================================================
// Bill Payments (Pagamento de Contas / Boletos)
// ============================================================

// BarcodeValidationRequest is sent to validate a barcode or digitable line.
type BarcodeValidationRequest struct {
	InputMethod   string `json:"input_method"`             // typed, pasted, camera_scan, file_upload
	Barcode       string `json:"barcode,omitempty"`        // 44 digits
	DigitableLine string `json:"digitable_line,omitempty"` // 47 or 48 digits
	ImageBase64   string `json:"image_base64,omitempty"`   // base64 image for camera_scan
}

// BarcodeValidationResponse contains validated barcode data.
type BarcodeValidationResponse struct {
	IsValid          bool     `json:"is_valid"`
	BillType         string   `json:"bill_type"` // bank_slip, utility, tax_slip, government
	Barcode          string   `json:"barcode,omitempty"`
	DigitableLine    string   `json:"digitable_line,omitempty"`
	BankCode         string   `json:"bank_code,omitempty"`
	Amount           float64  `json:"amount,omitempty"`
	DueDate          string   `json:"due_date,omitempty"`
	BeneficiaryName  string   `json:"beneficiary_name,omitempty"`
	BeneficiaryDoc   string   `json:"beneficiary_document,omitempty"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

// BillPaymentRequest is the payload to pay a bill/boleto.
type BillPaymentRequest struct {
	IdempotencyKey string  `json:"idempotency_key"`
	AccountID      string  `json:"account_id"`
	InputMethod    string  `json:"input_method"`
	Barcode        string  `json:"barcode,omitempty"`
	DigitableLine  string  `json:"digitable_line,omitempty"`
	Amount         float64 `json:"amount,omitempty"`         // override amount (if allowed)
	ScheduledDate  string  `json:"scheduled_date,omitempty"` // YYYY-MM-DD, empty = today
	Description    string  `json:"description,omitempty"`
}

// BillPayment represents a bill payment record.
type BillPayment struct {
	ID              string    `json:"id"`
	IdempotencyKey  string    `json:"idempotency_key"`
	CustomerID      string    `json:"customer_id"`
	AccountID       string    `json:"account_id"`
	InputMethod     string    `json:"input_method"`
	Barcode         string    `json:"barcode,omitempty"`
	DigitableLine   string    `json:"digitable_line,omitempty"`
	BillType        string    `json:"bill_type"`
	BeneficiaryName string    `json:"beneficiary_name,omitempty"`
	BeneficiaryDoc  string    `json:"beneficiary_document,omitempty"`
	OriginalAmount  float64   `json:"original_amount,omitempty"`
	FinalAmount     float64   `json:"final_amount"`
	DueDate         string    `json:"due_date,omitempty"`
	PaymentDate     string    `json:"payment_date,omitempty"`
	ScheduledDate   string    `json:"scheduled_date,omitempty"`
	Status          string    `json:"status"`
	FailureReason   string    `json:"failure_reason,omitempty"`
	ReceiptURL      string    `json:"receipt_url,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// ============================================================
// Debit Purchases
// ============================================================

// DebitPurchase represents a debit card purchase.
type DebitPurchase struct {
	ID              string    `json:"id"`
	CustomerID      string    `json:"customer_id"`
	AccountID       string    `json:"account_id"`
	TransactionDate time.Time `json:"transaction_date"`
	Amount          float64   `json:"amount"`
	MerchantName    string    `json:"merchant_name"`
	Category        string    `json:"category"`
	Description     string    `json:"description,omitempty"`
	CardLast4       string    `json:"card_last4,omitempty"`
	Status          string    `json:"status"`
	IsContactless   bool      `json:"is_contactless"`
}

// ============================================================
// Bill Payment API Response types (matches frontend spec)
// ============================================================

// BarcodeData is the nested validation data inside BarcodeValidationAPIResponse.
type BarcodeData struct {
	Barcode       string  `json:"barcode"`
	DigitableLine string  `json:"digitableLine"`
	Type          string  `json:"type"` // boleto, concessionaria, tributo
	Amount        float64 `json:"amount"`
	DueDate       string  `json:"dueDate,omitempty"`
	Beneficiary   string  `json:"beneficiary,omitempty"`
	Bank          string  `json:"bank,omitempty"`
	Discount      float64 `json:"discount"`
	Interest      float64 `json:"interest"`
	Fine          float64 `json:"fine"`
	TotalAmount   float64 `json:"totalAmount"`
}

// BarcodeValidationAPIResponse is the response for POST /v1/bills/validate.
type BarcodeValidationAPIResponse struct {
	Valid        bool         `json:"valid"`
	Data         *BarcodeData `json:"data,omitempty"`
	ErrorMessage string       `json:"errorMessage,omitempty"`
}

// BillPaymentAPIRequest is the body for POST /v1/bills/pay.
type BillPaymentAPIRequest struct {
	CustomerID  string `json:"customerId"`
	Barcode     string `json:"barcode"`
	InputMethod string `json:"inputMethod"` // camera, typed, pasted
	PaymentDate string `json:"paymentDate,omitempty"`
}

// BillPaymentAPIResponse is returned by bill payment endpoints.
type BillPaymentAPIResponse struct {
	TransactionID  string  `json:"transactionId"`
	Status         string  `json:"status"`
	Amount         float64 `json:"amount"`
	Beneficiary    string  `json:"beneficiary"`
	DueDate        string  `json:"dueDate,omitempty"`
	PaymentDate    string  `json:"paymentDate"`
	Authentication string  `json:"authentication"`
}

// DebitPurchaseRequest is the body for POST /v1/debit/purchase.
type DebitPurchaseRequest struct {
	CustomerID   string  `json:"customerId"`
	MerchantName string  `json:"merchantName"`
	Amount       float64 `json:"amount"`
	Category     string  `json:"category"`
	Description  string  `json:"description,omitempty"`
}

// DebitPurchaseResponse is returned by POST /v1/debit/purchase.
type DebitPurchaseResponse struct {
	TransactionID string  `json:"transactionId"`
	Status        string  `json:"status"` // completed, failed, insufficient_funds
	Amount        float64 `json:"amount"`
	NewBalance    float64 `json:"newBalance"`
	Timestamp     string  `json:"timestamp"`
}
