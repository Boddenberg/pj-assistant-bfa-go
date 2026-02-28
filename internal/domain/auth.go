package domain

import "time"

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
	CustomerID  string  `json:"customerId"`
	CreditLimit float64 `json:"creditLimit"`
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
	Success   bool    `json:"success"`
	Generated int     `json:"generated"`
	Income    float64 `json:"income"`
	Expenses  float64 `json:"expenses"`
	NetImpact float64 `json:"netImpact"`
	Message   string  `json:"message"`
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
