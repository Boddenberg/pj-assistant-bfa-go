package domain

import "time"

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
// Transactions (bank statement)
// ============================================================

// Transaction represents a single financial transaction.
type Transaction struct {
	ID           string    `json:"id"`
	Date         time.Time `json:"date"`
	Amount       float64   `json:"amount"`
	Type         string    `json:"type"` // pix_sent, pix_received, debit_purchase, credit_purchase, transfer_in, transfer_out, bill_payment, credit, debit
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
