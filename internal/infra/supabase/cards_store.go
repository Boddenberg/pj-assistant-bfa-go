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
// Credit Cards — CRUD via PostgREST
// ============================================================

func (c *Client) CreateCreditCard(ctx context.Context, customerID string, req *domain.CreditCardRequest) (*domain.CreditCard, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateCreditCard")
	defer span.End()

	// Generate a random last4 for demo
	last4 := fmt.Sprintf("%04d", time.Now().UnixNano()%10000)

	row := map[string]any{
		"customer_id":        customerID,
		"account_id":         req.AccountID,
		"card_number_last4":  last4,
		"card_holder_name":   customerID, // will be updated with real name
		"card_brand":         req.CardBrand,
		"card_type":          req.CardType,
		"credit_limit":       req.RequestedLimit,
		"available_limit":    req.RequestedLimit,
		"used_limit":         0,
		"billing_day":        req.BillingDay,
		"due_day":            req.DueDay,
		"status":             "active",
		"pix_credit_enabled": true,
		"pix_credit_limit":   req.RequestedLimit,
		"pix_credit_used":    0,
		"issued_at":          time.Now().Format(time.RFC3339),
		"expires_at":         time.Now().AddDate(5, 0, 0).Format(time.RFC3339),
	}

	body, err := c.doPost(ctx, "credit_cards", row)
	if err != nil {
		return nil, err
	}

	var results []domain.CreditCard
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode credit_card: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from credit_cards insert")
	}
	return &results[0], nil
}

func (c *Client) ListCreditCards(ctx context.Context, customerID string) ([]domain.CreditCard, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListCreditCards")
	defer span.End()

	path := fmt.Sprintf("credit_cards?customer_id=eq.%s&order=created_at.desc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCard
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode credit_cards: %w", err)
	}
	return rows, nil
}

func (c *Client) GetCreditCard(ctx context.Context, customerID, cardID string) (*domain.CreditCard, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCreditCard")
	defer span.End()

	path := fmt.Sprintf("credit_cards?id=eq.%s&limit=1", cardID)
	if customerID != "" {
		path = fmt.Sprintf("credit_cards?customer_id=eq.%s&id=eq.%s&limit=1", customerID, cardID)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCard
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode credit_card: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "credit_card", ID: cardID}
	}
	return &rows[0], nil
}

func (c *Client) UpdateCreditCardStatus(ctx context.Context, cardID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardStatus")
	defer span.End()

	patch := map[string]any{"status": status, "updated_at": time.Now().Format(time.RFC3339)}
	if status == "active" {
		patch["issued_at"] = time.Now().Format(time.RFC3339)
		patch["expires_at"] = time.Now().AddDate(5, 0, 0).Format(time.RFC3339)
	}

	return c.doPatch(ctx, fmt.Sprintf("credit_cards?id=eq.%s", cardID), patch)
}

// --- Credit Card Transactions ---

func (c *Client) ListCreditCardTransactions(ctx context.Context, customerID, cardID string, page, pageSize int) ([]domain.CreditCardTransaction, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListCreditCardTransactions")
	defer span.End()

	offset := (page - 1) * pageSize
	var path string
	if customerID != "" {
		path = fmt.Sprintf("credit_card_transactions?customer_id=eq.%s&card_id=eq.%s&order=transaction_date.desc,created_at.desc&limit=%d&offset=%d",
			customerID, cardID, pageSize, offset)
	} else {
		path = fmt.Sprintf("credit_card_transactions?card_id=eq.%s&order=transaction_date.desc,created_at.desc&limit=%d&offset=%d",
			cardID, pageSize, offset)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCardTransaction
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode cc_transactions: %w", err)
	}
	return rows, nil
}

// --- Credit Card Invoices ---

func (c *Client) ListCreditCardInvoices(ctx context.Context, customerID, cardID string) ([]domain.CreditCardInvoice, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListCreditCardInvoices")
	defer span.End()

	var path string
	if customerID != "" {
		path = fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&order=due_date.desc", customerID, cardID)
	} else {
		path = fmt.Sprintf("credit_card_invoices?card_id=eq.%s&order=due_date.desc", cardID)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCardInvoice
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode cc_invoices: %w", err)
	}
	return rows, nil
}

func (c *Client) GetCreditCardInvoice(ctx context.Context, customerID, cardID, invoiceID string) (*domain.CreditCardInvoice, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCreditCardInvoice")
	defer span.End()

	var path string
	if customerID != "" {
		path = fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&id=eq.%s&limit=1", customerID, cardID, invoiceID)
	} else {
		path = fmt.Sprintf("credit_card_invoices?card_id=eq.%s&id=eq.%s&limit=1", cardID, invoiceID)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCardInvoice
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode cc_invoice: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "credit_card_invoice", ID: invoiceID}
	}
	return &rows[0], nil
}

func (c *Client) GetCreditCardInvoiceByMonth(ctx context.Context, customerID, cardID, month string) (*domain.CreditCardInvoice, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCreditCardInvoiceByMonth")
	defer span.End()

	var path string
	if customerID != "" {
		path = fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&reference_month=eq.%s&limit=1", customerID, cardID, month)
	} else {
		path = fmt.Sprintf("credit_card_invoices?card_id=eq.%s&reference_month=eq.%s&limit=1", cardID, month)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.CreditCardInvoice
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode cc_invoice: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "credit_card_invoice", ID: month}
	}
	return &rows[0], nil
}

// --- Credit Card Limit / Used Limit Updates ---

func (c *Client) UpdateCreditCardLimit(ctx context.Context, customerID string, newLimit float64) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardLimit")
	defer span.End()

	// Get first card for customer
	cards, err := c.ListCreditCards(ctx, customerID)
	if err != nil {
		return err
	}
	if len(cards) == 0 {
		return &domain.ErrNotFound{Resource: "credit_card", ID: customerID}
	}

	card := cards[0]
	usedLimit := card.UsedLimit
	availableLimit := newLimit - usedLimit
	if availableLimit < 0 {
		availableLimit = 0
	}

	return c.doPatch(ctx, fmt.Sprintf("credit_cards?id=eq.%s", card.ID), map[string]any{
		"credit_limit":     newLimit,
		"available_limit":  availableLimit,
		"pix_credit_limit": newLimit,
	})
}

func (c *Client) InsertCreditCardTransaction(ctx context.Context, data map[string]any) error {
	ctx, span := tracer.Start(ctx, "Supabase.InsertCreditCardTransaction")
	defer span.End()

	_, err := c.doPost(ctx, "credit_card_transactions", data)
	return err
}

func (c *Client) UpdateCreditCardUsedLimit(ctx context.Context, cardID string, usedLimit, availableLimit float64) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardUsedLimit")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("credit_cards?id=eq.%s", cardID), map[string]any{
		"used_limit":      usedLimit,
		"available_limit": availableLimit,
	})
}

func (c *Client) UpdateCreditCardPixCreditUsed(ctx context.Context, cardID string, pixCreditUsed float64) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardPixCreditUsed")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("credit_cards?id=eq.%s", cardID), map[string]any{
		"pix_credit_used": pixCreditUsed,
	})
}

func (c *Client) UpdateCreditCardInvoiceStatus(ctx context.Context, invoiceID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardInvoiceStatus")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("credit_card_invoices?id=eq.%s", invoiceID), map[string]any{
		"status": status,
	})
}

// UpdateCreditCardInvoiceTotals updates totalAmount and minimumPayment on an invoice.
func (c *Client) UpdateCreditCardInvoiceTotals(ctx context.Context, invoiceID string, totalAmount, minimumPayment float64) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardInvoiceTotals")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("credit_card_invoices?id=eq.%s", invoiceID), map[string]any{
		"total_amount":    totalAmount,
		"minimum_payment": minimumPayment,
	})
}

func (c *Client) CreateCreditCardInvoice(ctx context.Context, invoice map[string]any) (*domain.CreditCardInvoice, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateCreditCardInvoice")
	defer span.End()

	body, err := c.doPost(ctx, "credit_card_invoices", invoice)
	if err != nil {
		return nil, err
	}

	var results []domain.CreditCardInvoice
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode cc_invoice: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from credit_card_invoices insert")
	}
	return &results[0], nil
}
