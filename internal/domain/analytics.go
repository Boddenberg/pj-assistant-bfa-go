package domain

import "time"

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
// Financial Summary & Analytics API types (matches frontend spec)
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
