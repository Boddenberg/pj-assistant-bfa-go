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
// PIX Transfers store — create, list, get, update status
// ============================================================

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
