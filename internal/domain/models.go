// Package domain defines the core business entities for the PJ Assistant.
// These models are independent of external services and represent the
// canonical data structures used throughout the BFA.
package domain

import "time"

// ============================================================
// Customer / Profile
// ============================================================

// CustomerProfile represents a PJ customer's profile data.
type CustomerProfile struct {
	CustomerID             string  `json:"customer_id"`
	Name                   string  `json:"name"`
	Document               string  `json:"document"` // CNPJ
	CompanyName            string  `json:"company_name,omitempty"`
	Email                  string  `json:"email,omitempty"`
	Segment                string  `json:"segment"`
	AccountStatus          string  `json:"account_status,omitempty"`
	RelationshipSince      string  `json:"relationship_since,omitempty"`
	MonthlyRevenue         float64 `json:"monthly_revenue"`
	AccountAge             int     `json:"account_age_months"`
	CreditScore            int     `json:"credit_score"`
	RepresentanteName      string  `json:"representante_name,omitempty"`
	RepresentanteCPF       string  `json:"representante_cpf,omitempty"`
	RepresentantePhone     string  `json:"representante_phone,omitempty"`
	RepresentanteBirthDate string  `json:"representante_birth_date,omitempty"`
}

// ============================================================
// Auth / Users
// ============================================================

// User represents an authenticated user (linked to Supabase Auth).
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone,omitempty"`
	FullName    string     `json:"full_name"`
	CPF         string     `json:"cpf,omitempty"`
	Role        string     `json:"role"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	IsActive    bool       `json:"is_active"`
	MFAEnabled  bool       `json:"mfa_enabled"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// UserCompany maps a user to a company (customer_profile) with a role.
type UserCompany struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	CustomerID  string   `json:"customer_id"`
	Role        string   `json:"role"`
	IsDefault   bool     `json:"is_default"`
	Permissions []string `json:"permissions"`
}

// ============================================================
// Accounts
// ============================================================

// Account represents a PJ bank account.
type Account struct {
	ID               string    `json:"id"`
	CustomerID       string    `json:"customer_id"`
	AccountType      string    `json:"account_type"`
	Branch           string    `json:"branch"`
	AccountNumber    string    `json:"account_number"`
	Digit            string    `json:"digit"`
	BankCode         string    `json:"bank_code"`
	BankName         string    `json:"bank_name"`
	Balance          float64   `json:"balance"`
	AvailableBalance float64   `json:"available_balance"`
	OverdraftLimit   float64   `json:"overdraft_limit"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// ============================================================
// Transactions (legacy — bank statement)
// ============================================================

// Transaction represents a single financial transaction.
type Transaction struct {
	ID           string    `json:"id"`
	Date         time.Time `json:"date"`
	Amount       float64   `json:"amount"`
	Type         string    `json:"type"` // credit, debit
	Category     string    `json:"category"`
	Description  string    `json:"description"`
	Counterparty string    `json:"counterparty,omitempty"`
}

// TransactionSummary provides aggregated transaction data.
type TransactionSummary struct {
	TotalCredits  float64         `json:"totalCredits"`
	TotalDebits   float64         `json:"totalDebits"`
	Balance       float64         `json:"balance"`
	Count         int             `json:"count"`
	Period        *SummaryPeriod  `json:"period,omitempty"`
	TopCategories []CategoryTotal `json:"top_categories,omitempty"`
}

// SummaryPeriod represents the date range for a summary.
type SummaryPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// CategoryTotal represents spending per category.
type CategoryTotal struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

// ============================================================
// PIX
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
	ScheduledFor           string  `json:"scheduled_for,omitempty"` // RFC3339 or empty for immediate
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
// Spending Analytics
// ============================================================

// SpendingSummary represents aggregated spending data for a period.
type SpendingSummary struct {
	ID                  string            `json:"id"`
	CustomerID          string            `json:"customer_id"`
	PeriodType          string            `json:"period_type"`  // daily, weekly, monthly, yearly
	PeriodStart         string            `json:"period_start"` // YYYY-MM-DD
	PeriodEnd           string            `json:"period_end"`
	TotalIncome         float64           `json:"total_income"`
	TotalExpenses       float64           `json:"total_expenses"`
	NetCashflow         float64           `json:"net_cashflow"`
	TransactionCount    int               `json:"transaction_count"`
	IncomeCount         int               `json:"income_count"`
	ExpenseCount        int               `json:"expense_count"`
	AvgIncome           float64           `json:"avg_income"`
	AvgExpense          float64           `json:"avg_expense"`
	LargestIncome       float64           `json:"largest_income"`
	LargestExpense      float64           `json:"largest_expense"`
	CategoryBreakdown   map[string]CatSum `json:"category_breakdown"`
	PixSentTotal        float64           `json:"pix_sent_total"`
	PixSentCount        int               `json:"pix_sent_count"`
	PixReceivedTotal    float64           `json:"pix_received_total"`
	PixReceivedCount    int               `json:"pix_received_count"`
	CreditCardTotal     float64           `json:"credit_card_total"`
	DebitCardTotal      float64           `json:"debit_card_total"`
	BillsPaidTotal      float64           `json:"bills_paid_total"`
	BillsPaidCount      int               `json:"bills_paid_count"`
	IncomeVariationPct  float64           `json:"income_variation_pct"`
	ExpenseVariationPct float64           `json:"expense_variation_pct"`
}

// CatSum is a spending breakdown per category.
type CatSum struct {
	Total float64 `json:"total"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct,omitempty"`
}

// SpendingBudget represents a monthly spending budget per category.
type SpendingBudget struct {
	ID                string  `json:"id"`
	CustomerID        string  `json:"customer_id"`
	Category          string  `json:"category"`
	MonthlyLimit      float64 `json:"monthly_limit"`
	AlertThresholdPct float64 `json:"alert_threshold_pct"`
	IsActive          bool    `json:"is_active"`
}

// ============================================================
// Favorites / Contacts
// ============================================================

// Favorite represents a saved payment recipient.
type Favorite struct {
	ID                string     `json:"id"`
	CustomerID        string     `json:"customer_id"`
	UserID            string     `json:"user_id"`
	Nickname          string     `json:"nickname"`
	DestinationType   string     `json:"destination_type"` // pix, ted, doc, bill
	PixKeyType        string     `json:"pix_key_type,omitempty"`
	PixKeyValue       string     `json:"pix_key_value,omitempty"`
	BankCode          string     `json:"bank_code,omitempty"`
	Branch            string     `json:"branch,omitempty"`
	AccountNumber     string     `json:"account_number,omitempty"`
	AccountType       string     `json:"account_type,omitempty"`
	RecipientName     string     `json:"recipient_name"`
	RecipientDocument string     `json:"recipient_document,omitempty"`
	UsageCount        int        `json:"usage_count"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
}

// ============================================================
// Transaction Limits
// ============================================================

// TransactionLimit represents configurable limits per transaction type.
type TransactionLimit struct {
	ID                 string   `json:"id"`
	CustomerID         string   `json:"customer_id"`
	TransactionType    string   `json:"transaction_type"`
	DailyLimit         float64  `json:"daily_limit"`
	DailyUsed          float64  `json:"daily_used"`
	MonthlyLimit       float64  `json:"monthly_limit"`
	MonthlyUsed        float64  `json:"monthly_used"`
	SingleLimit        float64  `json:"single_limit"`
	NightlySingleLimit *float64 `json:"nightly_single_limit,omitempty"`
	NightlyDailyLimit  *float64 `json:"nightly_daily_limit,omitempty"`
}

// ============================================================
// Notifications
// ============================================================

// Notification represents an in-app or push notification.
type Notification struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	CustomerID string     `json:"customer_id,omitempty"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Channel    string     `json:"channel"`
	Priority   string     `json:"priority"`
	IsRead     bool       `json:"is_read"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ============================================================
// AI Agent (kept from before)
// ============================================================

// AgentRequest is sent to the AI Agent service.
type AgentRequest struct {
	CustomerID   string              `json:"customer_id"`
	Profile      *CustomerProfile    `json:"profile"`
	Transactions []Transaction       `json:"transactions"`
	Summary      *TransactionSummary `json:"summary,omitempty"`
	Query        string              `json:"query,omitempty"`
}

// AgentResponse holds the AI Agent's structured response.
type AgentResponse struct {
	Answer        string     `json:"answer"`
	Reasoning     string     `json:"reasoning"`
	Sources       []string   `json:"sources,omitempty"`
	Confidence    float64    `json:"confidence"`
	TokensUsed    TokenUsage `json:"tokens_used"`
	ToolsExecuted []string   `json:"tools_executed,omitempty"`
}

// TokenUsage tracks LLM token consumption for cost monitoring.
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUsd float64 `json:"estimatedCostUsd,omitempty"`
}

// ============================================================
// Assistant API Request/Response (matches frontend spec)
// ============================================================

// AssistantRequest is the POST body for /v1/assistant/{customerId}.
type AssistantRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversationId,omitempty"`
}

// AssistantMessage represents a single chat message.
type AssistantMessage struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // user, assistant, system
	Content   string           `json:"content"`
	Timestamp string           `json:"timestamp"`
	Metadata  *MessageMetadata `json:"metadata,omitempty"`
}

// MessageMetadata enriches a message with tool/RAG/token info.
type MessageMetadata struct {
	ToolsUsed  []string    `json:"toolsUsed,omitempty"`
	RAGSources []RAGSource `json:"ragSources,omitempty"`
	TokenUsage *TokenUsage `json:"tokenUsage,omitempty"`
	LatencyMs  int64       `json:"latencyMs,omitempty"`
	Reasoning  string      `json:"reasoning,omitempty"`
}

// RAGSource represents a document source used by the RAG pipeline.
type RAGSource struct {
	DocumentID     string  `json:"documentId"`
	Title          string  `json:"title"`
	RelevanceScore float64 `json:"relevanceScore"`
	Snippet        string  `json:"snippet"`
}

// AssistantResponse is the final response from the BFA assistant endpoint.
type AssistantResponse struct {
	ConversationID string            `json:"conversationId"`
	Message        *AssistantMessage `json:"message"`
	Profile        *CustomerProfile  `json:"profile,omitempty"`
	Transactions   []Transaction     `json:"transactions,omitempty"`
}

// InternalAssistantResult is the service-level result before mapping to API shape.
type InternalAssistantResult struct {
	CustomerID     string
	Profile        *CustomerProfile
	Recommendation *AgentResponse
	ProcessedAt    time.Time
}

// ============================================================
// Health & Metrics API Responses
// ============================================================

// HealthStatus is returned by GET /healthz.
type HealthStatus struct {
	Status   string          `json:"status"` // healthy, degraded, unhealthy
	Services []ServiceHealth `json:"services"`
}

// ServiceHealth represents the health of an individual service.
type ServiceHealth struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	LatencyMs     int64   `json:"latencyMs"`
	UptimePercent float64 `json:"uptimePercent"`
	LastChecked   string  `json:"lastChecked"`
}

// AgentMetrics is returned by GET /v1/metrics/agent.
type AgentMetrics struct {
	TotalRequests       int64   `json:"totalRequests"`
	AvgLatencyMs        float64 `json:"avgLatencyMs"`
	P95LatencyMs        float64 `json:"p95LatencyMs"`
	P99LatencyMs        float64 `json:"p99LatencyMs"`
	ErrorRate           float64 `json:"errorRate"`
	FallbackRate        float64 `json:"fallbackRate"`
	AvgTokensPerRequest float64 `json:"avgTokensPerRequest"`
	EstimatedCostUsd    float64 `json:"estimatedCostUsd"`
	RAGPrecision        float64 `json:"ragPrecision"`
	CacheHitRate        float64 `json:"cacheHitRate"`
	Period              string  `json:"period"`
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
	Recipient     *PixRecipient `json:"recipient"`
	Timestamp     string        `json:"timestamp"`
	E2EID         string        `json:"e2eId"`
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
	Installments     int           `json:"installments"`
	InstallmentValue float64       `json:"installmentValue"`
	TotalWithFees    float64       `json:"totalWithFees"`
	Recipient        *PixRecipient `json:"recipient"`
	Timestamp        string        `json:"timestamp"`
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
	ID          string  `json:"id"`
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Installment string  `json:"installment,omitempty"`
	Category    string  `json:"category"`
}

// ============================================================
// Financial Summary & Debit API types (matches frontend spec)
// ============================================================

// FinancialSummary is returned by GET /v1/customers/{id}/financial/summary.
type FinancialSummary struct {
	CustomerID    string           `json:"customerId"`
	Period        *FinancialPeriod `json:"period"`
	Balance       *BalanceSummary  `json:"balance"`
	CashFlow      *CashFlowSummary `json:"cashFlow"`
	Spending      *SpendingDetail  `json:"spending"`
	TopCategories []TopCategory    `json:"topCategories"`
	MonthlyTrend  []MonthlyTrend   `json:"monthlyTrend"`
}

// FinancialPeriod is the time range for the financial summary.
type FinancialPeriod struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// BalanceSummary shows current balance breakdown.
type BalanceSummary struct {
	Current   float64 `json:"current"`
	Available float64 `json:"available"`
	Blocked   float64 `json:"blocked"`
	Invested  float64 `json:"invested"`
}

// CashFlowSummary shows income vs expenses.
type CashFlowSummary struct {
	TotalIncome              float64 `json:"totalIncome"`
	TotalExpenses            float64 `json:"totalExpenses"`
	NetCashFlow              float64 `json:"netCashFlow"`
	ComparedToPreviousPeriod float64 `json:"comparedToPreviousPeriod"`
}

// SpendingDetail shows spending analytics.
type SpendingDetail struct {
	TotalSpent               float64         `json:"totalSpent"`
	AverageDaily             float64         `json:"averageDaily"`
	HighestExpense           *HighestExpense `json:"highestExpense,omitempty"`
	ComparedToPreviousPeriod float64         `json:"comparedToPreviousPeriod"`
}

// HighestExpense represents the highest single expense.
type HighestExpense struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Date        string  `json:"date"`
	Category    string  `json:"category"`
}

// TopCategory is a spending category with trend.
type TopCategory struct {
	Category         string  `json:"category"`
	Amount           float64 `json:"amount"`
	Percentage       float64 `json:"percentage"`
	TransactionCount int     `json:"transactionCount"`
	Trend            string  `json:"trend"` // up, down, stable
}

// MonthlyTrend shows monthly income/expenses.
type MonthlyTrend struct {
	Month    string  `json:"month"`
	Income   float64 `json:"income"`
	Expenses float64 `json:"expenses"`
	Balance  float64 `json:"balance"`
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

// ============================================================
// Generic API Response wrappers
// ============================================================

// ListResponse wraps paginated list results.
type ListResponse[T any] struct {
	Data     []T  `json:"data"`
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasMore  bool `json:"has_more"`
}

// SuccessResponse wraps a successful single-entity response.
type SuccessResponse struct {
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}

// ============================================================
// Pix Key Registration — POST /v1/pix/keys/register
// ============================================================

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
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// ============================================================
// Invoice Payment — POST /v1/customers/:id/credit-cards/:cardId/invoice/pay
// ============================================================

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

// ============================================================
// Dev Tools — endpoints for development/testing
// ============================================================

// DevAddBalanceRequest is the body for POST /v1/dev/add-balance.
type DevAddBalanceRequest struct {
	CustomerID string  `json:"customerId"`
	Amount     float64 `json:"amount"`
}

// DevAddBalanceResponse is returned by POST /v1/dev/add-balance.
type DevAddBalanceResponse struct {
	Success    bool    `json:"success"`
	NewBalance float64 `json:"newBalance"`
	Message    string  `json:"message"`
}

// DevSetCreditLimitRequest is the body for POST /v1/dev/set-credit-limit.
type DevSetCreditLimitRequest struct {
	CustomerID string  `json:"customerId"`
	Limit      float64 `json:"limit"`
}

// DevSetCreditLimitResponse is returned by POST /v1/dev/set-credit-limit.
type DevSetCreditLimitResponse struct {
	Success  bool    `json:"success"`
	NewLimit float64 `json:"newLimit"`
	Message  string  `json:"message"`
}

// DevGenerateTransactionsRequest is the body for POST /v1/dev/generate-transactions.
type DevGenerateTransactionsRequest struct {
	CustomerID string `json:"customerId"`
	Count      int    `json:"count"`
	Months     int    `json:"months"` // how many months back to spread transactions (default 1, max 12)
}

// DevGenerateTransactionsResponse is returned by POST /v1/dev/generate-transactions.
type DevGenerateTransactionsResponse struct {
	Success   bool   `json:"success"`
	Generated int    `json:"generated"`
	Message   string `json:"message"`
}

// DevAddCardPurchaseRequest is the body for POST /v1/dev/add-card-purchase.
type DevAddCardPurchaseRequest struct {
	CustomerID  string  `json:"customerId"`
	CardID      string  `json:"cardId"`
	Amount      float64 `json:"amount"`
	Mode        string  `json:"mode"`        // "today" or "random"
	Count       int     `json:"count"`       // default 1
	TargetMonth string  `json:"targetMonth"` // optional, format "YYYY-MM" — generates purchases in that month
}

// DevAddCardPurchaseResponse is returned by POST /v1/dev/add-card-purchase.
type DevAddCardPurchaseResponse struct {
	Success     bool    `json:"success"`
	Generated   int     `json:"generated"`
	TotalAmount float64 `json:"totalAmount"`
	Message     string  `json:"message"`
}

// ============================================================
// Auth — Request / Response types (matches frontend API contract)
// ============================================================

// RegisterRequest is the body for POST /v1/auth/register.
type RegisterRequest struct {
	CNPJ                   string `json:"cnpj"`
	RazaoSocial            string `json:"razaoSocial"`
	NomeFantasia           string `json:"nomeFantasia"`
	Email                  string `json:"email"`
	RepresentanteName      string `json:"representanteName"`
	RepresentanteCPF       string `json:"representanteCpf"`
	RepresentantePhone     string `json:"representantePhone"`
	RepresentanteBirthDate string `json:"representanteBirthDate"`
	Password               string `json:"password"`
}

// RegisterResponse is the body for 201 from POST /v1/auth/register.
type RegisterResponse struct {
	CustomerID string `json:"customerId"`
	Agencia    string `json:"agencia"`
	Conta      string `json:"conta"`
	Message    string `json:"message"`
}

// LoginRequest is the body for POST /v1/auth/login.
type LoginRequest struct {
	CPF      string `json:"cpf"`
	Password string `json:"password"`
}

// LoginResponse is the body for 200 from POST /v1/auth/login.
type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	CustomerID   string `json:"customerId"`
	CustomerName string `json:"customerName"`
	CompanyName  string `json:"companyName"`
}

// RefreshRequest is the body for POST /v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// PasswordResetRequestBody is the body for POST /v1/auth/password/reset-request.
type PasswordResetRequestBody struct {
	Document string `json:"document"`
	Agencia  string `json:"agencia"`
	Conta    string `json:"conta"`
}

// PasswordResetRequestResponse is the response for reset-request.
type PasswordResetRequestResponse struct {
	Message     string `json:"message"`
	MaskedEmail string `json:"maskedEmail"`
	ExpiresIn   int    `json:"expiresIn"`
}

// PasswordResetConfirmRequest is the body for POST /v1/auth/password/reset-confirm.
type PasswordResetConfirmRequest struct {
	Document         string `json:"document"`
	Agencia          string `json:"agencia"`
	Conta            string `json:"conta"`
	VerificationCode string `json:"verificationCode"`
	NewPassword      string `json:"newPassword"`
}

// ChangePasswordRequest is the body for PUT /v1/auth/password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// UpdateProfileRequest is the body for PUT /v1/customers/{id}/profile.
type UpdateProfileRequest struct {
	NomeFantasia       string `json:"nomeFantasia,omitempty"`
	Email              string `json:"email,omitempty"`
	RepresentantePhone string `json:"representantePhone,omitempty"`
}

// UpdateProfileResponse is the response for profile update.
type UpdateProfileResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Document          string `json:"document"`
	CompanyName       string `json:"companyName"`
	Segment           string `json:"segment"`
	AccountStatus     string `json:"accountStatus"`
	RelationshipSince string `json:"relationshipSince"`
	CreditScore       int    `json:"creditScore"`
}

// UpdateRepresentativeRequest is the body for PUT /v1/customers/{id}/representative.
type UpdateRepresentativeRequest struct {
	RepresentanteName  string `json:"representanteName,omitempty"`
	RepresentantePhone string `json:"representantePhone,omitempty"`
}

// UpdateRepresentativeResponse is the response for representative update.
type UpdateRepresentativeResponse struct {
	Message                string `json:"message"`
	RepresentanteName      string `json:"representanteName"`
	RepresentanteCPF       string `json:"representanteCpf"`
	RepresentantePhone     string `json:"representantePhone"`
	RepresentanteBirthDate string `json:"representanteBirthDate"`
}

// AuthCredential represents stored credentials in the database.
type AuthCredential struct {
	ID                string     `json:"id"`
	CustomerID        string     `json:"customer_id"`
	PasswordHash      string     `json:"password_hash"`
	FailedAttempts    int        `json:"failed_attempts"`
	LockedUntil       *time.Time `json:"locked_until,omitempty"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt *time.Time `json:"password_changed_at,omitempty"`
}

// AuthRefreshToken represents a refresh token stored in the database.
type AuthRefreshToken struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	TokenHash  string    `json:"token_hash"`
	ExpiresAt  time.Time `json:"expires_at"`
	Revoked    bool      `json:"revoked"`
}

// AuthPasswordResetCode represents a password reset verification code.
type AuthPasswordResetCode struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Code       string    `json:"code"`
	ExpiresAt  time.Time `json:"expires_at"`
	Used       bool      `json:"used"`
}
