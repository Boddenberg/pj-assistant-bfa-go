package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"go.uber.org/zap"
)

// ============================================================
// BankingStore implementation — all banking CRUD via PostgREST
// ============================================================

// --- Accounts ---

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

// --- PIX Keys ---

func (c *Client) ListPixKeys(ctx context.Context, customerID string) ([]domain.PixKey, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListPixKeys")
	defer span.End()

	path := fmt.Sprintf("pix_keys?customer_id=eq.%s&status=eq.active", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixKey
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_keys: %w", err)
	}
	return rows, nil
}

func (c *Client) LookupPixKey(ctx context.Context, keyType, keyValue string) (*domain.PixKey, error) {
	ctx, span := tracer.Start(ctx, "Supabase.LookupPixKey")
	defer span.End()

	path := fmt.Sprintf("pix_keys?key_type=eq.%s&key_value=eq.%s&status=eq.active&limit=1", keyType, keyValue)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixKey
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_key lookup: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "pix_key", ID: keyValue}
	}
	return &rows[0], nil
}

// --- PIX Transfers ---

func (c *Client) CreatePixTransfer(ctx context.Context, customerID string, req *domain.PixTransferRequest) (*domain.PixTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreatePixTransfer")
	defer span.End()

	row := map[string]any{
		"idempotency_key":       req.IdempotencyKey,
		"source_account_id":     req.SourceAccountID,
		"source_customer_id":    customerID,
		"destination_key_type":  req.DestinationKeyType,
		"destination_key_value": req.DestinationKeyValue,
		"destination_name":      req.DestinationName,
		"destination_document":  req.DestinationDocument,
		"amount":                req.Amount,
		"description":           req.Description,
		"status":                "pending",
		"funded_by":             req.FundedBy,
	}
	if req.CreditCardID != "" {
		row["credit_card_id"] = req.CreditCardID
		row["credit_card_installments"] = req.CreditCardInstallments
	}
	if req.ScheduledFor != "" {
		row["scheduled_for"] = req.ScheduledFor
		row["status"] = "scheduled"
	}

	body, err := c.doPost(ctx, "pix_transfers", row)
	if err != nil {
		return nil, err
	}

	var results []domain.PixTransfer
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode pix_transfer: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result returned from pix_transfers insert")
	}
	return &results[0], nil
}

func (c *Client) ListPixTransfers(ctx context.Context, customerID string, page, pageSize int) ([]domain.PixTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListPixTransfers")
	defer span.End()

	offset := (page - 1) * pageSize
	path := fmt.Sprintf("pix_transfers?source_customer_id=eq.%s&order=created_at.desc&limit=%d&offset=%d",
		customerID, pageSize, offset)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixTransfer
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_transfers: %w", err)
	}
	return rows, nil
}

func (c *Client) GetPixTransfer(ctx context.Context, customerID, transferID string) (*domain.PixTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetPixTransfer")
	defer span.End()

	path := fmt.Sprintf("pix_transfers?source_customer_id=eq.%s&id=eq.%s&limit=1", customerID, transferID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixTransfer
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_transfer: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "pix_transfer", ID: transferID}
	}
	return &rows[0], nil
}

func (c *Client) UpdatePixTransferStatus(ctx context.Context, transferID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdatePixTransferStatus")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("pix_transfers?id=eq.%s", transferID), map[string]any{
		"status":     status,
		"updated_at": time.Now().Format(time.RFC3339),
	})
}

// --- Scheduled Transfers ---

func (c *Client) CreateScheduledTransfer(ctx context.Context, customerID string, req *domain.ScheduledTransferRequest) (*domain.ScheduledTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreateScheduledTransfer")
	defer span.End()

	row := map[string]any{
		"idempotency_key":          req.IdempotencyKey,
		"source_account_id":        req.SourceAccountID,
		"source_customer_id":       customerID,
		"transfer_type":            req.TransferType,
		"destination_bank_code":    req.DestinationBankCode,
		"destination_branch":       req.DestinationBranch,
		"destination_account":      req.DestinationAccount,
		"destination_account_type": req.DestinationAcctType,
		"destination_name":         req.DestinationName,
		"destination_document":     req.DestinationDocument,
		"amount":                   req.Amount,
		"description":              req.Description,
		"schedule_type":            req.ScheduleType,
		"scheduled_date":           req.ScheduledDate,
		"next_execution_date":      req.ScheduledDate,
		"status":                   "scheduled",
	}
	if req.RecurrenceEndDate != "" {
		row["recurrence_end_date"] = req.RecurrenceEndDate
	}
	if req.MaxRecurrences != nil {
		row["max_recurrences"] = *req.MaxRecurrences
	}

	body, err := c.doPost(ctx, "scheduled_transfers", row)
	if err != nil {
		return nil, err
	}

	var results []domain.ScheduledTransfer
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode scheduled_transfer: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from scheduled_transfers insert")
	}
	return &results[0], nil
}

func (c *Client) ListScheduledTransfers(ctx context.Context, customerID string) ([]domain.ScheduledTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListScheduledTransfers")
	defer span.End()

	path := fmt.Sprintf("scheduled_transfers?source_customer_id=eq.%s&order=scheduled_date.asc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.ScheduledTransfer
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode scheduled_transfers: %w", err)
	}
	return rows, nil
}

func (c *Client) GetScheduledTransfer(ctx context.Context, customerID, transferID string) (*domain.ScheduledTransfer, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetScheduledTransfer")
	defer span.End()

	path := fmt.Sprintf("scheduled_transfers?id=eq.%s&limit=1", transferID)
	if customerID != "" {
		path = fmt.Sprintf("scheduled_transfers?source_customer_id=eq.%s&id=eq.%s&limit=1", customerID, transferID)
	}
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.ScheduledTransfer
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode scheduled_transfer: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "scheduled_transfer", ID: transferID}
	}
	return &rows[0], nil
}

func (c *Client) UpdateScheduledTransferStatus(ctx context.Context, transferID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateScheduledTransferStatus")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("scheduled_transfers?id=eq.%s", transferID), map[string]any{
		"status":     status,
		"updated_at": time.Now().Format(time.RFC3339),
	})
}

// --- Credit Cards ---

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
		"status":             "pending_activation",
		"pix_credit_enabled": false,
		"pix_credit_limit":   0,
		"pix_credit_used":    0,
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
	path := fmt.Sprintf("credit_card_transactions?customer_id=eq.%s&card_id=eq.%s&order=transaction_date.desc&limit=%d&offset=%d",
		customerID, cardID, pageSize, offset)
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

	path := fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&order=due_date.desc", customerID, cardID)
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

	path := fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&id=eq.%s&limit=1", customerID, cardID, invoiceID)
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

	path := fmt.Sprintf("credit_card_invoices?customer_id=eq.%s&card_id=eq.%s&reference_month=eq.%s&limit=1", customerID, cardID, month)
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
	for _, t := range txns {
		if t.Type == "credit" {
			summary.TotalCredits += t.Amount
		} else {
			summary.TotalDebits += t.Amount
		}
	}
	summary.Balance = summary.TotalCredits - summary.TotalDebits

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

// --- Pix Key Registration ---

func (c *Client) CreatePixKey(ctx context.Context, key *domain.PixKey) (*domain.PixKey, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CreatePixKey")
	defer span.End()

	data := map[string]any{
		"id":          key.ID,
		"account_id":  key.AccountID,
		"customer_id": key.CustomerID,
		"key_type":    key.KeyType,
		"key_value":   key.KeyValue,
		"status":      "active",
	}

	body, err := c.doPost(ctx, "pix_keys", data)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixKey
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_keys: %w", err)
	}
	if len(rows) == 0 {
		return key, nil
	}
	return &rows[0], nil
}

// --- Account Balance Update (dev tools) ---

func (c *Client) UpdateAccountBalance(ctx context.Context, customerID string, delta float64) (*domain.Account, error) {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateAccountBalance")
	defer span.End()

	// Get primary account
	accts, err := c.ListAccounts(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if len(accts) == 0 {
		return nil, &domain.ErrNotFound{Resource: "account", ID: customerID}
	}

	acct := accts[0]
	newBalance := acct.Balance + delta
	newAvailable := acct.AvailableBalance + delta

	err = c.doPatch(ctx, fmt.Sprintf("accounts?id=eq.%s", acct.ID), map[string]any{
		"balance":           newBalance,
		"available_balance": newAvailable,
	})
	if err != nil {
		return nil, err
	}

	acct.Balance = newBalance
	acct.AvailableBalance = newAvailable
	return &acct, nil
}

// --- Credit Card Limit Update (dev tools) ---

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
		"credit_limit":   newLimit,
		"available_limit": availableLimit,
	})
}

// --- Invoice Status Update ---

func (c *Client) UpdateCreditCardInvoiceStatus(ctx context.Context, invoiceID, status string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpdateCreditCardInvoiceStatus")
	defer span.End()

	return c.doPatch(ctx, fmt.Sprintf("credit_card_invoices?id=eq.%s", invoiceID), map[string]any{
		"status": status,
	})
}

// ============================================================
// HTTP helpers for POST, PATCH, DELETE
// ============================================================

func (c *Client) doPost(ctx context.Context, table string, data map[string]any) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", c.baseURL, table)
	jsonBody, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceRoleKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("supabase: POST request failed",
			zap.String("table", table),
			zap.Error(err),
		)
		return nil, err
	}
	defer resp.Body.Close()

	body := make([]byte, 0)
	body, err = readBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Warn("supabase: POST non-2xx",
			zap.String("table", table),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)),
		)
		return nil, fmt.Errorf("supabase POST %s returned %d: %s", table, resp.StatusCode, string(body))
	}

	c.logger.Debug("supabase: POST OK", zap.String("table", table), zap.Int("status", resp.StatusCode))
	return body, nil
}

func (c *Client) doPatch(ctx context.Context, path string, data map[string]any) error {
	url := fmt.Sprintf("%s/rest/v1/%s", c.baseURL, path)
	jsonBody, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceRoleKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("supabase: PATCH request failed",
			zap.String("path", path),
			zap.Error(err),
		)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := readBody(resp)
		c.logger.Warn("supabase: PATCH non-2xx",
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)),
		)
		return fmt.Errorf("supabase PATCH returned %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("supabase: PATCH OK", zap.String("path", path))
	return nil
}

func (c *Client) doDelete(ctx context.Context, path string) error {
	url := fmt.Sprintf("%s/rest/v1/%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceRoleKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("supabase: DELETE request failed",
			zap.String("path", path),
			zap.Error(err),
		)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := readBody(resp)
		c.logger.Warn("supabase: DELETE non-2xx",
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)),
		)
		return fmt.Errorf("supabase DELETE returned %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("supabase: DELETE OK", zap.String("path", path))
	return nil
}

func readBody(resp *http.Response) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
