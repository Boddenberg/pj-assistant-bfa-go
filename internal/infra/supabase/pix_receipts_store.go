package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ============================================================
// PIX Receipts store — save, get, list
// ============================================================

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
