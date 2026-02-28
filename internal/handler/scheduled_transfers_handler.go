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
// Scheduled Transfers — create, delete, list
// ============================================================

func pixScheduleHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/schedule")
		defer span.End()

		var apiReq domain.PixScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		schedType := "once"
		recEndDate := ""
		if apiReq.Recurrence != nil {
			schedType = apiReq.Recurrence.Type
			recEndDate = apiReq.Recurrence.EndDate
		}

		req := &domain.ScheduledTransferRequest{
			IdempotencyKey:    uuid.New().String(),
			SourceAccountID:   account.ID,
			TransferType:      "pix",
			Amount:            apiReq.Amount,
			Description:       apiReq.Description,
			ScheduleType:      schedType,
			ScheduledDate:     apiReq.ScheduledDate,
			RecurrenceEndDate: recEndDate,
		}

		transfer, err := bankSvc.CreateScheduledTransfer(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.PixScheduleResponse{
			ScheduleID:    transfer.ID,
			Status:        transfer.Status,
			Amount:        transfer.Amount,
			ScheduledDate: transfer.ScheduledDate,
			Recipient: &domain.PixRecipient{
				Name: transfer.DestinationName,
			},
			Recurrence: apiReq.Recurrence,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func pixScheduleDeleteHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /v1/pix/schedule/{scheduleId}")
		defer span.End()

		scheduleID := chi.URLParam(r, "scheduleId")
		if err := bankSvc.CancelScheduledTransferByID(ctx, scheduleID); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func pixScheduledListHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/pix/scheduled")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		transfers, err := bankSvc.ListScheduledTransfers(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := make([]domain.PixScheduleResponse, 0, len(transfers))
		for _, t := range transfers {
			item := domain.PixScheduleResponse{
				ScheduleID:    t.ID,
				Status:        t.Status,
				Amount:        t.Amount,
				ScheduledDate: t.ScheduledDate,
				Recipient: &domain.PixRecipient{
					Name:     t.DestinationName,
					Document: t.DestinationDocument,
					Bank:     t.DestinationBankCode,
					Branch:   t.DestinationBranch,
					Account:  t.DestinationAccount,
				},
			}
			if t.ScheduleType != "once" {
				item.Recurrence = &domain.ScheduleRecurrence{Type: t.ScheduleType}
			}
			resp = append(resp, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"schedules": resp})
	}
}

// pixScheduledListByParamHandler is an alias using GET /v1/pix/scheduled/{customerId}
func pixScheduledListByParamHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return pixScheduledListHandler(bankSvc, logger)
}
