package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
)

// ============================================================
// PIX Keys, Transfers, Receipts, Scheduled Transfers
// ============================================================

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

// LookupPixKeyByValue searches for a pix key by value only (no key_type filter).
func (c *Client) LookupPixKeyByValue(ctx context.Context, keyValue string) (*domain.PixKey, error) {
	ctx, span := tracer.Start(ctx, "Supabase.LookupPixKeyByValue")
	defer span.End()

	path := fmt.Sprintf("pix_keys?key_value=eq.%s&status=eq.active&limit=1", keyValue)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixKey
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_key lookup by value: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "pix_key", ID: keyValue}
	}
	return &rows[0], nil
}

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

func (c *Client) DeletePixKey(ctx context.Context, customerID, keyID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.DeletePixKey")
	defer span.End()

	path := fmt.Sprintf("pix_keys?id=eq.%s&customer_id=eq.%s", keyID, customerID)
	if err := c.doDelete(ctx, path); err != nil {
		return err
	}
	return nil
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
		"end_to_end_id":         fmt.Sprintf("E%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:31]),
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

// --- PIX Receipts ---

func (c *Client) SavePixReceipt(ctx context.Context, receipt *domain.PixReceipt) (*domain.PixReceipt, error) {
	ctx, span := tracer.Start(ctx, "Supabase.SavePixReceipt")
	defer span.End()

	row := map[string]any{
		"id":                  receipt.ID,
		"transfer_id":         receipt.TransferID,
		"customer_id":         receipt.CustomerID,
		"direction":           receipt.Direction,
		"amount":              receipt.Amount,
		"original_amount":     receipt.OriginalAmount,
		"fee_amount":          receipt.FeeAmount,
		"total_amount":        receipt.TotalAmount,
		"description":         receipt.Description,
		"end_to_end_id":       receipt.EndToEndID,
		"funded_by":           receipt.FundedBy,
		"installments":        receipt.Installments,
		"sender_name":         receipt.SenderName,
		"sender_document":     receipt.SenderDocument,
		"sender_bank":         receipt.SenderBank,
		"sender_branch":       receipt.SenderBranch,
		"sender_account":      receipt.SenderAccount,
		"recipient_name":      receipt.RecipientName,
		"recipient_document":  receipt.RecipientDocument,
		"recipient_bank":      receipt.RecipientBank,
		"recipient_branch":    receipt.RecipientBranch,
		"recipient_account":   receipt.RecipientAccount,
		"recipient_key_type":  receipt.RecipientKeyType,
		"recipient_key_value": receipt.RecipientKeyValue,
		"transaction_id":      receipt.TransactionID,
		"status":              receipt.Status,
		"executed_at":         receipt.ExecutedAt,
		"created_at":          receipt.CreatedAt,
	}

	body, err := c.doPost(ctx, "pix_receipts", row)
	if err != nil {
		// If insert fails (possibly because fee columns don't exist yet), retry without them
		delete(row, "original_amount")
		delete(row, "fee_amount")
		delete(row, "total_amount")
		body, err = c.doPost(ctx, "pix_receipts", row)
		if err != nil {
			return nil, err
		}
	}

	var results []domain.PixReceipt
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode pix_receipt: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no result returned from pix_receipts insert")
	}
	return &results[0], nil
}

func (c *Client) GetPixReceipt(ctx context.Context, receiptID string) (*domain.PixReceipt, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetPixReceipt")
	defer span.End()

	path := fmt.Sprintf("pix_receipts?id=eq.%s&limit=1", receiptID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixReceipt
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_receipt: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "pix_receipt", ID: receiptID}
	}
	return &rows[0], nil
}

func (c *Client) GetPixReceiptByTransferID(ctx context.Context, transferID string) (*domain.PixReceipt, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetPixReceiptByTransferID")
	defer span.End()

	path := fmt.Sprintf("pix_receipts?transfer_id=eq.%s&limit=1", transferID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixReceipt
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_receipt by transfer_id: %w", err)
	}
	if len(rows) == 0 {
		return nil, &domain.ErrNotFound{Resource: "pix_receipt", ID: transferID}
	}
	return &rows[0], nil
}

func (c *Client) ListPixReceipts(ctx context.Context, customerID string) ([]domain.PixReceipt, error) {
	ctx, span := tracer.Start(ctx, "Supabase.ListPixReceipts")
	defer span.End()

	path := fmt.Sprintf("pix_receipts?customer_id=eq.%s&order=created_at.desc&limit=100", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var rows []domain.PixReceipt
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode pix_receipts: %w", err)
	}
	return rows, nil
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

// --- Customer Lookup (for PIX) ---

func (c *Client) GetCustomerName(ctx context.Context, customerID string) (string, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerName")
	defer span.End()

	path := fmt.Sprintf("customer_profiles?customer_id=eq.%s&select=company_name,name,representante_name&limit=1", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return "", err
	}

	var rows []struct {
		CompanyName       string `json:"company_name"`
		Name              string `json:"name"`
		RepresentanteName string `json:"representante_name"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return "", fmt.Errorf("decode customer_profiles: %w", err)
	}
	if len(rows) == 0 {
		return "Destinatário", nil
	}
	if rows[0].RepresentanteName != "" {
		return rows[0].RepresentanteName, nil
	}
	if rows[0].CompanyName != "" {
		return rows[0].CompanyName, nil
	}
	if rows[0].Name != "" {
		return rows[0].Name, nil
	}
	return "Destinatário", nil
}

// GetCustomerLookupData returns full profile + account data for pix lookup responses.
func (c *Client) GetCustomerLookupData(ctx context.Context, customerID string) (name, document, bank, branch, account string, err error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetCustomerLookupData")
	defer span.End()

	// 1. Get profile
	pPath := fmt.Sprintf("customer_profiles?customer_id=eq.%s&select=company_name,name,document,representante_name&limit=1", customerID)
	pBody, pErr := c.doRequest(ctx, http.MethodGet, pPath)
	if pErr != nil {
		err = pErr
		return
	}
	var profiles []struct {
		CompanyName       string `json:"company_name"`
		Name              string `json:"name"`
		Document          string `json:"document"`
		RepresentanteName string `json:"representante_name"`
	}
	if jErr := json.Unmarshal(pBody, &profiles); jErr != nil {
		err = jErr
		return
	}
	if len(profiles) > 0 {
		p := profiles[0]
		if p.RepresentanteName != "" {
			name = p.RepresentanteName
		} else if p.CompanyName != "" {
			name = p.CompanyName
		} else if p.Name != "" {
			name = p.Name
		} else {
			name = "Destinatário"
		}
		document = p.Document
	}

	// 2. Get account
	aPath := fmt.Sprintf("accounts?customer_id=eq.%s&status=eq.active&limit=1", customerID)
	aBody, aErr := c.doRequest(ctx, http.MethodGet, aPath)
	if aErr == nil {
		var accts []domain.Account
		if json.Unmarshal(aBody, &accts) == nil && len(accts) > 0 {
			bank = accts[0].BankName
			if bank == "" {
				bank = "Itaú Unibanco"
			}
			branch = accts[0].Branch
			account = accts[0].AccountNumber
			if accts[0].Digit != "" {
				account = accts[0].AccountNumber + "-" + accts[0].Digit
			}
		}
	}
	if bank == "" {
		bank = "Itaú Unibanco"
	}

	return
}
