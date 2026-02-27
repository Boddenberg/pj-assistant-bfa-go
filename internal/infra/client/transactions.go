package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/attribute"
)

// TransactionsClient fetches transaction data from the Transactions API.
type TransactionsClient struct {
	httpClient *http.Client
	baseURL    string
	cb         *gobreaker.CircuitBreaker
	cfg        resilience.Config
}

// NewTransactionsClient creates a new TransactionsClient.
func NewTransactionsClient(httpClient *http.Client, baseURL string, cb *gobreaker.CircuitBreaker, cfg resilience.Config) *TransactionsClient {
	return &TransactionsClient{
		httpClient: httpClient,
		baseURL:    baseURL,
		cb:         cb,
		cfg:        cfg,
	}
}

// GetTransactions fetches customer transactions with retry, circuit breaker, and tracing.
func (c *TransactionsClient) GetTransactions(ctx context.Context, customerID string) ([]domain.Transaction, error) {
	ctx, span := tracer.Start(ctx, "TransactionsClient.GetTransactions")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	var transactions []domain.Transaction

	result, err := c.cb.Execute(func() (any, error) {
		var innerErr error
		innerErr = resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			url := fmt.Sprintf("%s/v1/customers/%s/transactions", c.baseURL, customerID)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}

			resp, err := c.httpClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return &domain.ErrNotFound{Resource: "transactions", ID: customerID}
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("transactions API returned status %d", resp.StatusCode)
			}

			return json.NewDecoder(resp.Body).Decode(&transactions)
		})
		if innerErr != nil {
			return nil, innerErr
		}
		return transactions, nil
	})

	if err != nil {
		return nil, &domain.ErrExternalService{Service: "transactions", Err: err}
	}

	return result.([]domain.Transaction), nil
}
