package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/google/uuid"
)

// ============================================================
// AuthStore implementation — auth CRUD via PostgREST
// ============================================================

// --- Customer lookup ---

func (c *Client) GetCustomerByID(ctx context.Context, customerID string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerByID")
	defer span.End()

	path := fmt.Sprintf("customer_profiles?customer_id=eq.%s&limit=1", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil
	}

	var rows []domain.CustomerProfile
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode customer_profiles: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (c *Client) GetCustomerByDocument(ctx context.Context, document string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerByDocument")
	defer span.End()

	path := fmt.Sprintf("customer_profiles?document=eq.%s&limit=1", document)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil // not found is not an error for auth lookup
	}

	var rows []domain.CustomerProfile
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode customer_profiles: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (c *Client) GetCustomerByCPF(ctx context.Context, cpf string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerByCPF")
	defer span.End()

	path := fmt.Sprintf("customer_profiles?representante_cpf=eq.%s&limit=1", cpf)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil
	}

	var rows []domain.CustomerProfile
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode customer_profiles: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (c *Client) GetCustomerByBankDetails(ctx context.Context, document, agencia, conta string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerByBankDetails")
	defer span.End()

	// First find the customer by document
	profile, err := c.GetCustomerByDocument(ctx, document)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, nil
	}

	// Then verify the account belongs to this customer with matching agencia + conta
	path := fmt.Sprintf("accounts?customer_id=eq.%s&branch=eq.%s&account_number=eq.%s&limit=1",
		profile.CustomerID, agencia, conta)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil // no matching account
	}

	var accounts []domain.Account
	if err := json.Unmarshal(body, &accounts); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}
	if len(accounts) == 0 {
		return nil, nil
	}

	return profile, nil
}

// --- Registration ---

func (c *Client) CreateCustomerWithAccount(ctx context.Context, req *domain.RegisterRequest, passwordHash string) (*domain.RegisterResponse, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateCustomerWithAccount")
	defer span.End()

	customerID := uuid.New().String()
	agencia := fmt.Sprintf("%04d", rand.Intn(10000))
	conta := fmt.Sprintf("%07d-%d", rand.Intn(10000000), rand.Intn(10))

	// 1. Create customer profile
	profileData := map[string]any{
		"id":                      customerID,
		"customer_id":             customerID,
		"name":                    req.RepresentanteName,
		"document":                req.CNPJ,
		"segment":                 "pj_standard",
		"monthly_revenue":         0,
		"account_age_months":      0,
		"credit_score":            700,
		"company_name":            req.NomeFantasia,
		"email":                   req.Email,
		"account_status":          "active",
		"relationship_since":      time.Now().Format("2006-01-02"),
		"representante_name":      req.RepresentanteName,
		"representante_cpf":       req.RepresentanteCPF,
		"representante_phone":     req.RepresentantePhone,
		"representante_birth_date": req.RepresentanteBirthDate,
	}

	_, err := c.doPost(ctx, "customer_profiles", profileData)
	if err != nil {
		return nil, fmt.Errorf("create customer profile: %w", err)
	}

	// 2. Create account
	accountData := map[string]any{
		"id":                uuid.New().String(),
		"customer_id":       customerID,
		"account_type":      "checking",
		"account_number":    conta,
		"branch":            agencia,
		"digit":             "0",
		"balance":           0,
		"available_balance": 0,
		"overdraft_limit":   0,
		"currency":          "BRL",
		"status":            "active",
	}

	_, err = c.doPost(ctx, "accounts", accountData)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	// 3. Create auth credentials
	credData := map[string]any{
		"id":              uuid.New().String(),
		"customer_id":     customerID,
		"password_hash":   passwordHash,
		"failed_attempts": 0,
	}

	_, err = c.doPost(ctx, "auth_credentials", credData)
	if err != nil {
		return nil, fmt.Errorf("create auth credentials: %w", err)
	}

	return &domain.RegisterResponse{
		CustomerID: customerID,
		Agencia:    agencia,
		Conta:      conta,
		Message:    "Conta criada com sucesso",
	}, nil
}

// --- Credentials ---

func (c *Client) GetCredentials(ctx context.Context, customerID string) (*domain.AuthCredential, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCredentials")
	defer span.End()

	path := fmt.Sprintf("auth_credentials?customer_id=eq.%s&limit=1", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, &domain.ErrNotFound{Resource: "credentials", ID: customerID}
	}

	var rows []domain.AuthCredential
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode auth_credentials: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "credentials", ID: customerID}
	}
	return &rows[0], nil
}

func (c *Client) UpdateCredentials(ctx context.Context, customerID string, updates map[string]any) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCredentials")
	defer span.End()

	path := fmt.Sprintf("auth_credentials?customer_id=eq.%s", customerID)
	return c.doPatch(ctx, path, updates)
}

// --- Refresh tokens ---

func (c *Client) StoreRefreshToken(ctx context.Context, customerID, tokenHash string, expiresAt time.Time) error {
	ctx, span := tracer.Start(ctx, "Supabase.StoreRefreshToken")
	defer span.End()

	data := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"token_hash":  tokenHash,
		"expires_at":  expiresAt.Format(time.RFC3339),
		"revoked":     false,
	}

	_, err := c.doPost(ctx, "auth_refresh_tokens", data)
	return err
}

func (c *Client) GetRefreshToken(ctx context.Context, tokenHash string) (*domain.AuthRefreshToken, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetRefreshToken")
	defer span.End()

	path := fmt.Sprintf("auth_refresh_tokens?token_hash=eq.%s&revoked=eq.false&limit=1", tokenHash)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil
	}

	var rows []domain.AuthRefreshToken
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode auth_refresh_tokens: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (c *Client) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	ctx, span := tracer.Start(ctx, "Supabase.RevokeRefreshToken")
	defer span.End()

	path := fmt.Sprintf("auth_refresh_tokens?token_hash=eq.%s", tokenHash)
	return c.doPatch(ctx, path, map[string]any{"revoked": true})
}

func (c *Client) RevokeAllRefreshTokens(ctx context.Context, customerID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.RevokeAllRefreshTokens")
	defer span.End()

	path := fmt.Sprintf("auth_refresh_tokens?customer_id=eq.%s&revoked=eq.false", customerID)
	return c.doPatch(ctx, path, map[string]any{"revoked": true})
}

// --- Password reset codes ---

func (c *Client) StoreResetCode(ctx context.Context, customerID, code string, expiresAt time.Time) error {
	ctx, span := tracer.Start(ctx, "Supabase.StoreResetCode")
	defer span.End()

	data := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"code":        code,
		"expires_at":  expiresAt.Format(time.RFC3339),
		"used":        false,
	}

	_, err := c.doPost(ctx, "auth_password_reset_codes", data)
	return err
}

func (c *Client) GetValidResetCode(ctx context.Context, customerID, code string) (*domain.AuthPasswordResetCode, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetValidResetCode")
	defer span.End()

	now := time.Now().UTC().Format(time.RFC3339)
	path := fmt.Sprintf("auth_password_reset_codes?customer_id=eq.%s&code=eq.%s&used=eq.false&expires_at=gt.%s&order=created_at.desc&limit=1",
		customerID, code, now)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil
	}

	var rows []domain.AuthPasswordResetCode
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode auth_password_reset_codes: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (c *Client) MarkResetCodeUsed(ctx context.Context, codeID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.MarkResetCodeUsed")
	defer span.End()

	path := fmt.Sprintf("auth_password_reset_codes?id=eq.%s", codeID)
	return c.doPatch(ctx, path, map[string]any{"used": true})
}

// --- Profile updates ---

func (c *Client) UpdateCustomerProfile(ctx context.Context, customerID string, updates map[string]any) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCustomerProfile")
	defer span.End()

	path := fmt.Sprintf("customer_profiles?customer_id=eq.%s", customerID)
	if err := c.doPatch(ctx, path, updates); err != nil {
		return nil, err
	}

	// Re-fetch updated profile
	fetchPath := fmt.Sprintf("customer_profiles?customer_id=eq.%s&limit=1", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, fetchPath)
	if err != nil {
		return nil, err
	}

	var rows []domain.CustomerProfile
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode customer_profiles: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "customer_profile", ID: customerID}
	}
	return &rows[0], nil
}

func (c *Client) UpdateRepresentative(ctx context.Context, customerID string, updates map[string]any) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateRepresentative")
	defer span.End()

	// Representative fields are stored in customer_profiles
	return c.UpdateCustomerProfile(ctx, customerID, updates)
}
