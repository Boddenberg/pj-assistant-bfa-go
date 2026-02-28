package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ============================================================
// Customer Lookup store — name + full lookup data (used by PIX)
// ============================================================

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
