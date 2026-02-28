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
// Scheduled Transfers store — create, list, get, update status
// ============================================================

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
