package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

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
