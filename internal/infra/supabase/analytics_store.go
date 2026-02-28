package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ============================================================
// Analytics — Spending, Budgets, Favorites, Limits, Notifications, Transactions
// ============================================================

// --- Transaction Summary ---

func (c *Client) GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetTransactionSummary")
	defer span.End()

	path := fmt.Sprintf("customer_transactions?customer_id=eq.%s&order=date.desc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var txns []domain.Transaction
	if err := json.Unmarshal(body, &txns); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}

	summary := &domain.TransactionSummary{Count: len(txns)}
	categoryTotals := make(map[string]float64)
	for _, t := range txns {
		// Skip devtools transactions from financial calculations —
		// they are balance adjustments, not real income/expenses.
		if t.Category == "devtools" {
			continue
		}
		if t.Amount >= 0 {
			summary.TotalCredits += t.Amount
		} else {
			summary.TotalDebits += -t.Amount // store as positive
			// Accumulate expense by category
			if t.Category != "" {
				categoryTotals[t.Category] += -t.Amount
			}
		}
	}
	summary.Balance = summary.TotalCredits - summary.TotalDebits

	// Build top categories from expense breakdown
	topCats := make([]domain.CategoryTotal, 0, len(categoryTotals))
	for cat, total := range categoryTotals {
		topCats = append(topCats, domain.CategoryTotal{Category: cat, Total: total})
	}
	// Sort by total descending
	for i := 0; i < len(topCats); i++ {
		for j := i + 1; j < len(topCats); j++ {
			if topCats[i].Total < topCats[j].Total {
				topCats[i], topCats[j] = topCats[j], topCats[i]
			}
		}
	}
	summary.TopCategories = topCats

	if len(txns) > 0 {
		summary.Period = &domain.SummaryPeriod{
			From: txns[len(txns)-1].Date.Format("2006-01-02"),
			To:   txns[0].Date.Format("2006-01-02"),
		}
	}

	return summary, nil
}

// InsertTransaction inserts a raw transaction record (used by dev tools).
func (c *Client) InsertTransaction(ctx context.Context, data map[string]any) error {
	ctx, span := tracer.Start(ctx, "Supabase.InsertTransaction")
	defer span.End()

	_, err := c.doPost(ctx, "customer_transactions", data)
	return err
}

// ListTransactions returns transactions for a customer within a date range.
func (c *Client) ListTransactions(ctx context.Context, customerID string, from, to string) ([]domain.Transaction, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListTransactions")
	defer span.End()

	path := fmt.Sprintf("customer_transactions?customer_id=eq.%s&date=gte.%s&date=lt.%s&order=date.desc&limit=1000",
		customerID, from, to)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var txns []domain.Transaction
	if err := json.Unmarshal(body, &txns); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}
	return txns, nil
}

// --- Spending Analytics ---

func (c *Client) GetSpendingSummary(ctx context.Context, customerID, periodType string) (*domain.SpendingSummary, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetSpendingSummary")
	defer span.End()

	path := fmt.Sprintf("spending_summaries?customer_id=eq.%s&period_type=eq.%s&order=period_start.desc&limit=1",
		customerID, periodType)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, &domain.ErrNotFound{Resource: "spending_summary", ID: customerID}
	}

	var rows []domain.SpendingSummary
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode spending_summary: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "spending_summary", ID: customerID}
	}
	return &rows[0], nil
}

// --- Budgets ---

func (c *Client) ListBudgets(ctx context.Context, customerID string) ([]domain.SpendingBudget, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListBudgets")
	defer span.End()

	path := fmt.Sprintf("spending_budgets?customer_id=eq.%s&is_active=eq.true", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.SpendingBudget
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode spending_budgets: %w", err)
	}
	return rows, nil
}

func (c *Client) CreateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateBudget")
	defer span.End()

	row := map[string]any{
		"customer_id":         budget.CustomerID,
		"category":            budget.Category,
		"monthly_limit":       budget.MonthlyLimit,
		"alert_threshold_pct": budget.AlertThresholdPct,
		"is_active":           budget.IsActive,
	}

	body, err := c.doPost(ctx, "spending_budgets", row)
	if err != nil {
		return nil, err
	}

	var results []domain.SpendingBudget
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode spending_budget: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from spending_budgets insert")
	}
	return &results[0], nil
}

func (c *Client) UpdateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateBudget")
	defer span.End()

	err := c.doPatch(ctx, fmt.Sprintf("spending_budgets?id=eq.%s&customer_id=eq.%s", budget.ID, budget.CustomerID), map[string]any{
		"monthly_limit":       budget.MonthlyLimit,
		"alert_threshold_pct": budget.AlertThresholdPct,
		"is_active":           budget.IsActive,
		"updated_at":          time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	return budget, nil
}

// --- Favorites ---

func (c *Client) ListFavorites(ctx context.Context, customerID string) ([]domain.Favorite, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListFavorites")
	defer span.End()

	path := fmt.Sprintf("favorites?customer_id=eq.%s&order=usage_count.desc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.Favorite
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode favorites: %w", err)
	}
	return rows, nil
}

func (c *Client) CreateFavorite(ctx context.Context, fav *domain.Favorite) (*domain.Favorite, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateFavorite")
	defer span.End()

	row := map[string]any{
		"customer_id":        fav.CustomerID,
		"user_id":            fav.UserID,
		"nickname":           fav.Nickname,
		"destination_type":   fav.DestinationType,
		"pix_key_type":       fav.PixKeyType,
		"pix_key_value":      fav.PixKeyValue,
		"bank_code":          fav.BankCode,
		"branch":             fav.Branch,
		"account_number":     fav.AccountNumber,
		"account_type":       fav.AccountType,
		"recipient_name":     fav.RecipientName,
		"recipient_document": fav.RecipientDocument,
	}

	body, err := c.doPost(ctx, "favorites", row)
	if err != nil {
		return nil, err
	}

	var results []domain.Favorite
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode favorite: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from favorites insert")
	}
	return &results[0], nil
}

func (c *Client) DeleteFavorite(ctx context.Context, customerID, favoriteID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.DeleteFavorite")
	defer span.End()

	return c.doDelete(ctx, fmt.Sprintf("favorites?id=eq.%s&customer_id=eq.%s", favoriteID, customerID))
}

// --- Transaction Limits ---

func (c *Client) ListTransactionLimits(ctx context.Context, customerID string) ([]domain.TransactionLimit, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListTransactionLimits")
	defer span.End()

	path := fmt.Sprintf("transaction_limits?customer_id=eq.%s", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.TransactionLimit
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode transaction_limits: %w", err)
	}
	return rows, nil
}

func (c *Client) GetTransactionLimit(ctx context.Context, customerID, txType string) (*domain.TransactionLimit, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetTransactionLimit")
	defer span.End()

	path := fmt.Sprintf("transaction_limits?customer_id=eq.%s&transaction_type=eq.%s&limit=1", customerID, txType)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.TransactionLimit
	if body != nil {
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("decode transaction_limit: %w", err)
		}
	}
	if len(rows) == 0 {
		return nil, nil // no limit configured = no restriction
	}
	return &rows[0], nil
}

func (c *Client) UpdateTransactionLimit(ctx context.Context, limit *domain.TransactionLimit) (*domain.TransactionLimit, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateTransactionLimit")
	defer span.End()

	err := c.doPatch(ctx,
		fmt.Sprintf("transaction_limits?customer_id=eq.%s&transaction_type=eq.%s", limit.CustomerID, limit.TransactionType),
		map[string]any{
			"daily_limit":   limit.DailyLimit,
			"monthly_limit": limit.MonthlyLimit,
			"single_limit":  limit.SingleLimit,
			"updated_at":    time.Now().Format(time.RFC3339),
		})
	if err != nil {
		return nil, err
	}
	return limit, nil
}

// --- Notifications ---

func (c *Client) ListNotifications(ctx context.Context, customerID string, unreadOnly bool, page, pageSize int) ([]domain.Notification, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListNotifications")
	defer span.End()

	offset := (page - 1) * pageSize
	path := fmt.Sprintf("notifications?customer_id=eq.%s&order=created_at.desc&limit=%d&offset=%d",
		customerID, pageSize, offset)
	if unreadOnly {
		path += "&is_read=eq.false"
	}

	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.Notification
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode notifications: %w", err)
	}
	return rows, nil
}

func (c *Client) MarkNotificationRead(ctx context.Context, notifID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.MarkNotificationRead")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("notifications?id=eq.%s", notifID), map[string]any{
		"is_read": true,
		"read_at": time.Now().Format(time.RFC3339),
	})
}
