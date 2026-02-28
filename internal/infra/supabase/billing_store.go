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
// Bill Payments & Debit Purchases — CRUD via PostgREST
// ============================================================

// --- Bill Payments ---

func (c *Client) CreateBillPayment(ctx context.Context, customerID string, req *domain.BillPaymentRequest, validation *domain.BarcodeValidationResponse) (*domain.BillPayment, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateBillPayment")
	defer span.End()

	amount := req.Amount
	if amount == 0 {
		amount = validation.Amount
	}

	status := "pending"
	if req.ScheduledDate != "" {
		status = "scheduled"
	}

	row := map[string]any{
		"idempotency_key":      req.IdempotencyKey,
		"customer_id":          customerID,
		"account_id":           req.AccountID,
		"input_method":         req.InputMethod,
		"barcode":              validation.Barcode,
		"digitable_line":       validation.DigitableLine,
		"bill_type":            validation.BillType,
		"beneficiary_name":     validation.BeneficiaryName,
		"beneficiary_document": validation.BeneficiaryDoc,
		"original_amount":      validation.Amount,
		"final_amount":         amount,
		"due_date":             validation.DueDate,
		"payment_date":         time.Now().Format("2006-01-02"),
		"scheduled_date":       req.ScheduledDate,
		"status":               status,
	}

	body, err := c.doPost(ctx, "bill_payments", row)
	if err != nil {
		return nil, err
	}

	var results []domain.BillPayment
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode bill_payment: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from bill_payments insert")
	}
	return &results[0], nil
}

func (c *Client) ListBillPayments(ctx context.Context, customerID string, page, pageSize int) ([]domain.BillPayment, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListBillPayments")
	defer span.End()

	offset := (page - 1) * pageSize
	path := fmt.Sprintf("bill_payments?customer_id=eq.%s&order=created_at.desc&limit=%d&offset=%d",
		customerID, pageSize, offset)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.BillPayment
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode bill_payments: %w", err)
	}
	return rows, nil
}

func (c *Client) GetBillPayment(ctx context.Context, customerID, billID string) (*domain.BillPayment, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetBillPayment")
	defer span.End()

	path := fmt.Sprintf("bill_payments?customer_id=eq.%s&id=eq.%s&limit=1", customerID, billID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.BillPayment
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode bill_payment: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "bill_payment", ID: billID}
	}
	return &rows[0], nil
}

func (c *Client) UpdateBillPaymentStatus(ctx context.Context, billID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateBillPaymentStatus")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("bill_payments?id=eq.%s", billID), map[string]any{
		"status":     status,
		"updated_at": time.Now().Format(time.RFC3339),
	})
}

// --- Debit Purchases ---

func (c *Client) ListDebitPurchases(ctx context.Context, customerID string, page, pageSize int) ([]domain.DebitPurchase, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListDebitPurchases")
	defer span.End()

	offset := (page - 1) * pageSize
	path := fmt.Sprintf("debit_purchases?customer_id=eq.%s&order=transaction_date.desc&limit=%d&offset=%d",
		customerID, pageSize, offset)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.DebitPurchase
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode debit_purchases: %w", err)
	}
	return rows, nil
}

func (c *Client) CreateDebitPurchase(ctx context.Context, customerID string, req *domain.DebitPurchaseRequest) (*domain.DebitPurchase, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateDebitPurchase")
	defer span.End()

	// Get primary account
	accounts, err := c.ListAccounts(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, &domain.ErrNotFound{Resource: "account", ID: customerID}
	}
	accountID := accounts[0].ID

	row := map[string]any{
		"customer_id":      customerID,
		"account_id":       accountID,
		"transaction_date": time.Now().Format(time.RFC3339),
		"amount":           req.Amount,
		"merchant_name":    req.MerchantName,
		"category":         req.Category,
		"description":      req.Description,
		"status":           "completed",
		"is_contactless":   false,
	}

	body, err := c.doPost(ctx, "debit_purchases", row)
	if err != nil {
		return nil, err
	}

	var results []domain.DebitPurchase
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode debit_purchase: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from debit_purchases insert")
	}
	return &results[0], nil
}
