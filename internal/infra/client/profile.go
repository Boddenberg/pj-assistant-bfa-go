package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("client")

// ProfileClient fetches customer profile data from the Profile API.
type ProfileClient struct {
	httpClient *http.Client
	baseURL    string
	cb         *gobreaker.CircuitBreaker
	cfg        resilience.Config
}

// NewProfileClient creates a new ProfileClient.
func NewProfileClient(httpClient *http.Client, baseURL string, cb *gobreaker.CircuitBreaker, cfg resilience.Config) *ProfileClient {
	return &ProfileClient{
		httpClient: httpClient,
		baseURL:    baseURL,
		cb:         cb,
		cfg:        cfg,
	}
}

// GetProfile fetches a customer profile with retry, circuit breaker, and tracing.
func (c *ProfileClient) GetProfile(ctx context.Context, customerID string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "ProfileClient.GetProfile")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	var profile domain.CustomerProfile

	result, err := c.cb.Execute(func() (any, error) {
		var innerErr error
		innerErr = resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			url := fmt.Sprintf("%s/v1/customers/%s/profile", c.baseURL, customerID)
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
				return &domain.ErrNotFound{Resource: "profile", ID: customerID}
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("profile API returned status %d", resp.StatusCode)
			}

			return json.NewDecoder(resp.Body).Decode(&profile)
		})
		if innerErr != nil {
			return nil, innerErr
		}
		return &profile, nil
	})

	if err != nil {
		return nil, &domain.ErrExternalService{Service: "profile", Err: err}
	}

	return result.(*domain.CustomerProfile), nil
}
