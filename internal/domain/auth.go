package domain

import "time"

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
