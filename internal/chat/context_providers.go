package chat

import (
	"context"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"
	"go.uber.org/zap"
)

/*
 * Context Providers — buscam dados financeiros do cliente para envio ao Agent Python.
 *
 * Cada provider é independente e falha silenciosamente (log + nil).
 * O orquestrador BuildFinancialContext chama todos em paralelo e
 * monta o FinancialContext agregado.
 *
 * A latência total é limitada pelo provider mais lento, não pela soma.
 */

const contextTimeout = 3 * time.Second

// ContextFetcher abstrai o acesso aos dados financeiros.
// Em produção é satisfeito pelo BankingStore (supabaseClient).
type ContextFetcher interface {
	port.AccountStore
	port.PixKeyStore
	port.PixTransferStore
	port.ScheduledTransferStore
	port.CreditCardStore
	port.CreditCardInvoiceStore
	port.BillingStore
	port.CustomerLookupStore
}

// BuildFinancialContext busca todos os sub-contextos do cliente.
// Cada chamada é feita de forma independente; erros isolados são logados
// mas não impedem os demais contextos de serem preenchidos.
func BuildFinancialContext(ctx context.Context, store ContextFetcher, authStore port.AuthStore, customerID string, logger *zap.Logger) *FinancialContext {
	ctx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()

	fc := &FinancialContext{
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		ContextKeys: []string{},
	}

	// Account
	if acc := fetchAccountContext(ctx, store, customerID, logger); acc != nil {
		fc.Account = acc
		fc.ContextKeys = append(fc.ContextKeys, "account")
	}

	// Cards + Invoices
	if cards := fetchCardsContext(ctx, store, customerID, logger); cards != nil {
		fc.Cards = cards
		fc.ContextKeys = append(fc.ContextKeys, "cards")
	}

	// PIX
	if pix := fetchPixContext(ctx, store, customerID, logger); pix != nil {
		fc.Pix = pix
		fc.ContextKeys = append(fc.ContextKeys, "pix")
	}

	// Billing
	if billing := fetchBillingContext(ctx, store, customerID, logger); billing != nil {
		fc.Billing = billing
		fc.ContextKeys = append(fc.ContextKeys, "billing")
	}

	// Profile
	if profile := fetchProfileContext(ctx, authStore, customerID, logger); profile != nil {
		fc.Profile = profile
		fc.ContextKeys = append(fc.ContextKeys, "profile")
	}

	if len(fc.ContextKeys) == 0 {
		logger.Warn("financial context: nenhum sub-contexto preenchido",
			zap.String("customer_id", customerID),
		)
		return nil
	}

	logger.Info("📦 financial context montado",
		zap.String("customer_id", customerID),
		zap.Strings("keys", fc.ContextKeys),
	)

	return fc
}

/* ---------- Individual providers ---------- */

func fetchAccountContext(ctx context.Context, store port.AccountStore, customerID string, logger *zap.Logger) *AccountContext {
	acc, err := store.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		logger.Debug("financial context: account fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	return &AccountContext{
		AccountID:            acc.ID,
		Branch:               acc.Branch,
		AccountNumber:        acc.AccountNumber,
		Balance:              acc.Balance,
		AvailableBalance:     acc.AvailableBalance,
		OverdraftLimit:       acc.OverdraftLimit,
		CreditLimit:          acc.CreditLimit,
		AvailableCreditLimit: acc.AvailableCreditLimit,
		Status:               acc.Status,
	}
}

func fetchCardsContext(ctx context.Context, store interface {
	port.CreditCardStore
	port.CreditCardInvoiceStore
}, customerID string, logger *zap.Logger) *CardsContext {
	cards, err := store.ListCreditCards(ctx, customerID)
	if err != nil {
		logger.Debug("financial context: cards fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	if len(cards) == 0 {
		return nil
	}

	cc := &CardsContext{
		Cards: make([]CardSummary, len(cards)),
	}
	for i, c := range cards {
		cc.Cards[i] = CardSummary{
			CardID:         c.ID,
			Last4:          c.CardNumberLast4,
			Brand:          c.CardBrand,
			CardType:       c.CardType,
			Status:         c.Status,
			CreditLimit:    c.CreditLimit,
			AvailableLimit: c.AvailableLimit,
			UsedLimit:      c.UsedLimit,
			DueDay:         c.DueDay,
			BillingDay:     c.BillingDay,
		}

		// Buscar faturas abertas/pendentes para cada cartão
		invoices, err := store.ListCreditCardInvoices(ctx, customerID, c.ID)
		if err != nil {
			logger.Debug("financial context: invoices fetch failed",
				zap.String("card_id", c.ID),
				zap.Error(err),
			)
			continue
		}
		for _, inv := range invoices {
			if inv.Status == "open" || inv.Status == "overdue" || inv.Status == "closed" {
				cc.Invoices = append(cc.Invoices, InvoiceSummary{
					CardID:         c.ID,
					ReferenceMonth: inv.ReferenceMonth,
					TotalAmount:    inv.TotalAmount,
					MinimumPayment: inv.MinimumPayment,
					DueDate:        inv.DueDate,
					Status:         inv.Status,
				})
			}
		}
	}

	return cc
}

func fetchPixContext(ctx context.Context, store interface {
	port.PixKeyStore
	port.PixTransferStore
	port.ScheduledTransferStore
}, customerID string, logger *zap.Logger) *PixContext {
	pc := &PixContext{}

	// Chaves PIX
	keys, err := store.ListPixKeys(ctx, customerID)
	if err != nil {
		logger.Debug("financial context: pix keys fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
	} else {
		pc.Keys = make([]PixKeySummary, len(keys))
		for i, k := range keys {
			pc.Keys[i] = PixKeySummary{
				KeyType:  k.KeyType,
				KeyValue: k.KeyValue,
				Status:   k.Status,
			}
		}
	}

	// Transferências recentes (page 1, 10 itens)
	transfers, err := store.ListPixTransfers(ctx, customerID, 1, 10)
	if err != nil {
		logger.Debug("financial context: pix transfers fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
	} else {
		for _, t := range transfers {
			pc.RecentTransfers = append(pc.RecentTransfers, PixTransferSummary{
				TransferID:      t.ID,
				Amount:          t.Amount,
				DestinationName: t.DestinationName,
				Status:          t.Status,
				FundedBy:        t.FundedBy,
				CreatedAt:       t.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	// Transferências agendadas
	scheduled, err := store.ListScheduledTransfers(ctx, customerID)
	if err != nil {
		logger.Debug("financial context: scheduled transfers fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
	} else {
		for _, st := range scheduled {
			pc.ScheduledTransfers = append(pc.ScheduledTransfers, ScheduledSummary{
				TransferID:      st.ID,
				Amount:          st.Amount,
				DestinationName: st.DestinationName,
				ScheduledFor:    st.ScheduledDate,
				Status:          st.Status,
			})
		}
	}

	// Se nenhum dado foi encontrado, retornar nil
	if len(pc.Keys) == 0 && len(pc.RecentTransfers) == 0 && len(pc.ScheduledTransfers) == 0 {
		return nil
	}

	return pc
}

func fetchBillingContext(ctx context.Context, store port.BillingStore, customerID string, logger *zap.Logger) *BillingContext {
	bc := &BillingContext{}

	// Boletos recentes (page 1, 10 itens)
	bills, err := store.ListBillPayments(ctx, customerID, 1, 10)
	if err != nil {
		logger.Debug("financial context: bills fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
	} else {
		for _, b := range bills {
			bc.RecentBills = append(bc.RecentBills, BillSummary{
				BillID:      b.ID,
				Amount:      b.FinalAmount,
				Beneficiary: b.BeneficiaryName,
				DueDate:     b.DueDate,
				Status:      b.Status,
			})
		}
	}

	// Compras no débito recentes (page 1, 10 itens)
	debits, err := store.ListDebitPurchases(ctx, customerID, 1, 10)
	if err != nil {
		logger.Debug("financial context: debit purchases fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
	} else {
		for _, d := range debits {
			bc.RecentDebits = append(bc.RecentDebits, DebitSummary{
				Amount:       d.Amount,
				MerchantName: d.MerchantName,
				Category:     d.Category,
				Date:         d.TransactionDate.Format("2006-01-02"),
				Status:       d.Status,
			})
		}
	}

	if len(bc.RecentBills) == 0 && len(bc.RecentDebits) == 0 {
		return nil
	}

	return bc
}

func fetchProfileContext(ctx context.Context, store port.AuthStore, customerID string, logger *zap.Logger) *ProfileContext {
	profile, err := store.GetCustomerByID(ctx, customerID)
	if err != nil {
		logger.Debug("financial context: profile fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	companyName := profile.CompanyName
	if companyName == "" {
		companyName = profile.Name
	}
	return &ProfileContext{
		CustomerID:  profile.CustomerID,
		CompanyName: companyName,
		Document:    profile.Document,
		Segment:     profile.Segment,
		Email:       profile.Email,
	}
}
