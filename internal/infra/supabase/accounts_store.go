package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
)

// ============================================================
// Accounts — CRUD via PostgREST
// ============================================================

func (c *Client) ListAccounts(ctx context.Context, customerID string) ([]domain.Account, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListAccounts")
	defer span.End()

	path := fmt.Sprintf("accounts?customer_id=eq.%s&order=created_at.asc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.Account
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}
	return rows, nil
}

func (c *Client) GetAccount(ctx context.Context, customerID, accountID string) (*domain.Account, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetAccount")
	defer span.End()

	path := fmt.Sprintf("accounts?customer_id=eq.%s&id=eq.%s&limit=1", customerID, accountID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.Account
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "account", ID: accountID}
	}
	return &rows[0], nil
}

func (c *Client) GetPrimaryAccount(ctx context.Context, customerID string) (*domain.Account, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetPrimaryAccount")
	defer span.End()

	path := fmt.Sprintf("accounts?customer_id=eq.%s&status=eq.active&order=created_at.asc&limit=1", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.Account
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "account", ID: customerID}
	}
	return &rows[0], nil
}

// UpdateAccountBalance adjusts the primary account balance by a delta.
func (c *Client) UpdateAccountBalance(ctx context.Context, customerID string, delta float64) (*domain.Account, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateAccountBalance")
	defer span.End()

	// Get primary (active) account — never use ListAccounts which may return inactive accounts
	acct, err := c.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		return nil, err
	}

	newBalance := acct.Balance + delta
	newAvailable := acct.AvailableBalance + delta

	err = c.doPatch(ctx, fmt.Sprintf("accounts?id=eq.%s", acct.ID), map[string]any{
		"balance":           newBalance,
		"available_balance": newAvailable,
	})
	if err != nil {
		return nil, err
	}

	// Re-fetch to confirm the update actually persisted
	updated, err := c.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("re-fetch after balance update: %w", err)
	}

	c.logger.Info("supabase: balance updated",
		zap.String("account_id", updated.ID),
		zap.Float64("old_balance", acct.Balance),
		zap.Float64("new_balance", updated.Balance),
	)

	return updated, nil
}
