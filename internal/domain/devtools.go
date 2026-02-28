package domain

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
	CustomerID   string  `json:"customerId"`
	CreditCardID string  `json:"creditCardId,omitempty"`
	CreditLimit  float64 `json:"creditLimit"`
}

// DevSetCreditLimitResponse is returned by POST /v1/dev/set-credit-limit.
type DevSetCreditLimitResponse struct {
	Success  bool    `json:"success"`
	NewLimit float64 `json:"newLimit"`
	Message  string  `json:"message"`
}

// DevGenerateTransactionsRequest is the body for POST /v1/dev/generate-transactions.
type DevGenerateTransactionsRequest struct {
	CustomerID   string `json:"customerId"`
	Count        int    `json:"count"`
	Months       int    `json:"months"`       // how many months back to spread transactions (default 1, max 12)
	Period       string `json:"period"`       // "current-month" or "last-12-months" (overrides months if set)
	ApplyBalance bool   `json:"applyBalance"` // if true, also update account balance with net impact (default false)
}

// DevGenerateTransactionsResponse is returned by POST /v1/dev/generate-transactions.
type DevGenerateTransactionsResponse struct {
	Success      bool          `json:"success"`
	Generated    int           `json:"generated"`
	Income       float64       `json:"income"`
	Expenses     float64       `json:"expenses"`
	NetImpact    float64       `json:"netImpact"`
	Message      string        `json:"message"`
	Transactions []Transaction `json:"transactions"`
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
