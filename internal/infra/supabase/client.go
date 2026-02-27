// Package supabase provides a client for Supabase (PostgREST + Auth).
// Used as the real data backend for customer profiles and transactions.
package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("supabase")

// Client wraps HTTP calls to Supabase PostgREST API.
type Client struct {
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	serviceRoleKey string
	cb             *gobreaker.CircuitBreaker
	cfg            resilience.Config
	logger         *zap.Logger
}

// NewClient creates a Supabase client.
func NewClient(httpClient *http.Client, baseURL, apiKey, serviceRoleKey string, cb *gobreaker.CircuitBreaker, cfg resilience.Config, logger *zap.Logger) *Client {
	return &Client{
		httpClient:     httpClient,
		baseURL:        baseURL,
		apiKey:         apiKey,
		serviceRoleKey: serviceRoleKey,
		cb:             cb,
		cfg:            cfg,
		logger:         logger,
	}
}

// doRequest executes an authenticated request to Supabase PostgREST.
func (c *Client) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		c.logger.Error("supabase: failed to create request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Error(err),
		)
		return nil, err
	}

	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceRoleKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("supabase: request failed",
			zap.String("method", method),
			zap.String("path", path),
			zap.Error(err),
		)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("supabase: failed to read response body",
			zap.String("method", method),
			zap.String("path", path),
			zap.Error(err),
		)
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, nil // no data
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Warn("supabase: non-2xx response",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)),
		)
		return nil, fmt.Errorf("supabase returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("supabase: request OK",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status", resp.StatusCode),
	)

	return body, nil
}

// --- Profile API (implements port.ProfileFetcher) ---

// supabaseProfile maps Supabase table columns to our domain.
type supabaseProfile struct {
	ID             string  `json:"id"`
	CustomerID     string  `json:"customer_id"`
	Name           string  `json:"name"`
	Document       string  `json:"document"`
	Segment        string  `json:"segment"`
	MonthlyRevenue float64 `json:"monthly_revenue"`
	AccountAge     int     `json:"account_age_months"`
	CreditScore    int     `json:"credit_score"`
}

// GetProfile fetches customer profile from Supabase.
func (c *Client) GetProfile(ctx context.Context, customerID string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetProfile")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	var profile *domain.CustomerProfile

	_, err := c.cb.Execute(func() (any, error) {
		return nil, resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			path := fmt.Sprintf("customer_profiles?customer_id=eq.%s&limit=1", customerID)
			body, err := c.doRequest(ctx, http.MethodGet, path)
			if err != nil {
				return err
			}

			if body == nil || string(body) == "[]" {
				return &domain.ErrNotFound{Resource: "profile", ID: customerID}
			}

			var profiles []supabaseProfile
			if err := json.Unmarshal(body, &profiles); err != nil {
				return fmt.Errorf("failed to decode profile: %w", err)
			}

			if len(profiles) == 0 {
				return &domain.ErrNotFound{Resource: "profile", ID: customerID}
			}

			p := profiles[0]
			profile = &domain.CustomerProfile{
				CustomerID:     p.CustomerID,
				Name:           p.Name,
				Document:       p.Document,
				Segment:        p.Segment,
				MonthlyRevenue: p.MonthlyRevenue,
				AccountAge:     p.AccountAge,
				CreditScore:    p.CreditScore,
			}
			return nil
		})
	})

	if err != nil {
		return nil, &domain.ErrExternalService{Service: "supabase/profile", Err: err}
	}

	return profile, nil
}

// --- Transactions API (implements port.TransactionsFetcher) ---

// supabaseTransaction maps Supabase table columns.
type supabaseTransaction struct {
	ID          string  `json:"id"`
	CustomerID  string  `json:"customer_id"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
}

// GetTransactions fetches customer transactions from Supabase.
func (c *Client) GetTransactions(ctx context.Context, customerID string) ([]domain.Transaction, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetTransactions")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	var transactions []domain.Transaction

	_, err := c.cb.Execute(func() (any, error) {
		return nil, resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			path := fmt.Sprintf("customer_transactions?customer_id=eq.%s&order=date.desc&limit=100", customerID)
			body, err := c.doRequest(ctx, http.MethodGet, path)
			if err != nil {
				return err
			}

			if body == nil || string(body) == "[]" {
				transactions = []domain.Transaction{}
				return nil
			}

			var rows []supabaseTransaction
			if err := json.Unmarshal(body, &rows); err != nil {
				return fmt.Errorf("failed to decode transactions: %w", err)
			}

			transactions = make([]domain.Transaction, 0, len(rows))
			for _, r := range rows {
				t, _ := time.Parse(time.RFC3339, r.Date)
				if t.IsZero() {
					t, _ = time.Parse("2006-01-02", r.Date)
				}
				transactions = append(transactions, domain.Transaction{
					ID:          r.ID,
					Date:        t,
					Amount:      r.Amount,
					Type:        r.Type,
					Category:    r.Category,
					Description: r.Description,
				})
			}
			return nil
		})
	})

	if err != nil {
		return nil, &domain.ErrExternalService{Service: "supabase/transactions", Err: err}
	}

	return transactions, nil
}
