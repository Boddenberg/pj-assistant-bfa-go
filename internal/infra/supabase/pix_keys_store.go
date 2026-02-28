package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ============================================================
// PIX Keys store — list, lookup, create, delete
// ============================================================

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
