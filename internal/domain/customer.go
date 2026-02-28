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
