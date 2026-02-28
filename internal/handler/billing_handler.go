package handler

import (
	"encoding/json"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================
// 6. Pagamento de Boletos
// ============================================================

func billsValidateHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/bills/validate")
		defer span.End()

		var body struct {
			Barcode string `json:"barcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		valReq := &domain.BarcodeValidationRequest{
			InputMethod:   "typed",
			DigitableLine: body.Barcode,
			Barcode:       body.Barcode,
		}

		result, err := bankSvc.ValidateBarcode(ctx, valReq)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.BarcodeValidationAPIResponse{Valid: result.IsValid}
		if result.IsValid {
			billType := result.BillType
			switch billType {
			case "bank_slip":
				billType = "boleto"
			case "utility":
				billType = "concessionaria"
			}
			resp.Data = &domain.BarcodeData{
				Barcode:       result.Barcode,
				DigitableLine: result.DigitableLine,
				Type:          billType,
				Amount:        result.Amount,
				DueDate:       result.DueDate,
				Beneficiary:   result.BeneficiaryName,
				Bank:          result.BankCode,
				TotalAmount:   result.Amount,
			}
		} else {
			if len(result.ValidationErrors) > 0 {
				resp.ErrorMessage = result.ValidationErrors[0]
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func billsPayHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/bills/pay")
		defer span.End()

		var apiReq domain.BillPaymentAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		req := &domain.BillPaymentRequest{
			IdempotencyKey: uuid.New().String(),
			AccountID:      account.ID,
			InputMethod:    apiReq.InputMethod,
			DigitableLine:  apiReq.Barcode,
			Barcode:        apiReq.Barcode,
			ScheduledDate:  apiReq.PaymentDate,
		}

		payment, err := bankSvc.PayBill(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.BillPaymentAPIResponse{
			TransactionID:  payment.ID,
			Status:         payment.Status,
			Amount:         payment.FinalAmount,
			Beneficiary:    payment.BeneficiaryName,
			DueDate:        payment.DueDate,
			PaymentDate:    payment.PaymentDate,
			Authentication: payment.IdempotencyKey,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func billsHistoryHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/bills/history")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		page, pageSize := parsePagination(r)

		payments, err := bankSvc.ListBillPayments(ctx, customerID, page, pageSize)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := make([]domain.BillPaymentAPIResponse, 0, len(payments))
		for _, p := range payments {
			resp = append(resp, domain.BillPaymentAPIResponse{
				TransactionID:  p.ID,
				Status:         p.Status,
				Amount:         p.FinalAmount,
				Beneficiary:    p.BeneficiaryName,
				DueDate:        p.DueDate,
				PaymentDate:    p.PaymentDate,
				Authentication: p.IdempotencyKey,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ============================================================
// 8b. Débito
// ============================================================

func debitPurchaseHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/debit/purchase")
		defer span.End()

		var apiReq domain.DebitPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.CreateDebitPurchase(ctx, apiReq.CustomerID, &apiReq)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}
