package chat

import (
	"context"
	"sync"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"
	"go.uber.org/zap"
)

/*
 * Context Providers — buscam dados financeiros do cliente para envio ao Agent Python.
 *
 * Cada provider é independente e falha silenciosamente (log + nil).
 * O orquestrador BuildFinancialContext chama todos em PARALELO e
 * monta o FinancialContext agregado.
 *
 * A latência total é limitada pelo provider mais lento, não pela soma.
 */

const contextTimeout = 15 * time.Second

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
	port.AnalyticsStore
}

// BuildFinancialContext busca todos os sub-contextos do cliente.
// Cada chamada é feita de forma independente; erros isolados são logados
// mas não impedem os demais contextos de serem preenchidos.
func BuildFinancialContext(ctx context.Context, store ContextFetcher, authStore port.AuthStore, customerID string, logger *zap.Logger) *FinancialContext {
	all := []string{"account", "cards", "pix", "billing", "profile", "analytics", "transactions"}
	return BuildSelectiveContext(ctx, store, authStore, customerID, all, logger)
}

// BuildSelectiveContext busca os sub-contextos listados em requiredContexts em PARALELO.
// Cada provider roda em sua própria goroutine — a latência total é a do mais lento, não a soma.
// Erros isolados são logados mas não impedem os demais providers.
func BuildSelectiveContext(ctx context.Context, store ContextFetcher, authStore port.AuthStore, customerID string, requiredContexts []string, logger *zap.Logger) *FinancialContext {
	if len(requiredContexts) == 0 {
		return nil
	}

	// NÃO criamos um único timeout compartilhado aqui.
	// Cada goroutine abaixo cria seu próprio ctx com timeout independente,
	// assim um provider lento não mata o deadline dos outros.

	fc := &FinancialContext{
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		ContextKeys: []string{},
	}

	// Montar set para lookup O(1)
	need := make(map[string]bool, len(requiredContexts))
	for _, k := range requiredContexts {
		need[k] = true
	}

	var mu sync.Mutex // protege fc
	var wg sync.WaitGroup

	if need["account"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if acc := fetchAccountContext(pCtx, store, customerID, logger); acc != nil {
				mu.Lock()
				fc.Account = acc
				fc.ContextKeys = append(fc.ContextKeys, "account")
				mu.Unlock()
			}
		}()
	}

	if need["cards"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if cards := fetchCardsContext(pCtx, store, customerID, logger); cards != nil {
				mu.Lock()
				fc.Cards = cards
				fc.ContextKeys = append(fc.ContextKeys, "cards")
				mu.Unlock()
			}
		}()
	}

	if need["pix"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if pix := fetchPixContext(pCtx, store, customerID, logger); pix != nil {
				mu.Lock()
				fc.Pix = pix
				fc.ContextKeys = append(fc.ContextKeys, "pix")
				mu.Unlock()
			}
		}()
	}

	if need["billing"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if billing := fetchBillingContext(pCtx, store, customerID, logger); billing != nil {
				mu.Lock()
				fc.Billing = billing
				fc.ContextKeys = append(fc.ContextKeys, "billing")
				mu.Unlock()
			}
		}()
	}

	if need["profile"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if profile := fetchProfileContext(pCtx, authStore, customerID, logger); profile != nil {
				mu.Lock()
				fc.Profile = profile
				fc.ContextKeys = append(fc.ContextKeys, "profile")
				mu.Unlock()
			}
		}()
	}

	if need["analytics"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if analytics := fetchAnalyticsContext(pCtx, store, customerID, logger); analytics != nil {
				mu.Lock()
				fc.Analytics = analytics
				fc.ContextKeys = append(fc.ContextKeys, "analytics")
				mu.Unlock()
			}
		}()
	}

	if need["transactions"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pCtx, pCancel := context.WithTimeout(ctx, contextTimeout)
			defer pCancel()
			if txns := fetchTransactionsContext(pCtx, store, customerID, logger); txns != nil {
				mu.Lock()
				fc.Transactions = txns
				fc.ContextKeys = append(fc.ContextKeys, "transactions")
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if len(fc.ContextKeys) == 0 {
		logger.Warn("financial context: nenhum sub-contexto preenchido",
			zap.String("customer_id", customerID),
		)
		return nil
	}

	logger.Info("📦 financial context montado",
		zap.String("customer_id", customerID),
		zap.Strings("required", requiredContexts),
		zap.Strings("fetched", fc.ContextKeys),
	)

	return fc
}

/* ---------- Individual providers ---------- */

func fetchAccountContext(ctx context.Context, store port.AccountStore, customerID string, logger *zap.Logger) *AccountContext {
	acc, err := store.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		logger.Warn("financial context: account fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	if acc == nil {
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
		logger.Warn("financial context: cards fetch failed",
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
			logger.Warn("financial context: invoices fetch failed",
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
		logger.Warn("financial context: pix keys fetch failed",
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
		logger.Warn("financial context: pix transfers fetch failed",
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
		logger.Warn("financial context: scheduled transfers fetch failed",
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
		logger.Warn("financial context: bills fetch failed",
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
		logger.Warn("financial context: debit purchases fetch failed",
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
		logger.Warn("financial context: profile fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	if profile == nil {
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

func fetchAnalyticsContext(ctx context.Context, store port.AnalyticsStore, customerID string, logger *zap.Logger) *AnalyticsContext {
	summary, err := store.GetSpendingSummary(ctx, customerID, "monthly")
	if err != nil {
		logger.Warn("financial context: analytics fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	if summary == nil {
		return nil
	}

	// Converter category breakdown
	cats := make(map[string]CategoryTotal, len(summary.CategoryBreakdown))
	for k, v := range summary.CategoryBreakdown {
		cats[k] = CategoryTotal{
			Total: v.Total,
			Count: v.Count,
			Pct:   v.Pct,
		}
	}

	return &AnalyticsContext{
		PeriodType:          summary.PeriodType,
		PeriodStart:         summary.PeriodStart,
		PeriodEnd:           summary.PeriodEnd,
		TotalIncome:         summary.TotalIncome,
		TotalExpenses:       summary.TotalExpenses,
		NetCashflow:         summary.NetCashflow,
		TransactionCount:    summary.TransactionCount,
		IncomeCount:         summary.IncomeCount,
		ExpenseCount:        summary.ExpenseCount,
		AvgIncome:           summary.AvgIncome,
		AvgExpense:          summary.AvgExpense,
		LargestIncome:       summary.LargestIncome,
		LargestExpense:      summary.LargestExpense,
		CategoryBreakdown:   cats,
		PixSentTotal:        summary.PixSentTotal,
		PixSentCount:        summary.PixSentCount,
		PixReceivedTotal:    summary.PixReceivedTotal,
		PixReceivedCount:    summary.PixReceivedCount,
		CreditCardTotal:     summary.CreditCardTotal,
		DebitCardTotal:      summary.DebitCardTotal,
		BillsPaidTotal:      summary.BillsPaidTotal,
		BillsPaidCount:      summary.BillsPaidCount,
		IncomeVariationPct:  summary.IncomeVariationPct,
		ExpenseVariationPct: summary.ExpenseVariationPct,
	}
}

func fetchTransactionsContext(ctx context.Context, store port.AnalyticsStore, customerID string, logger *zap.Logger) *TransactionsContext {
	// Buscar transações dos últimos 90 dias (limite de 30 para não estourar o payload)
	now := time.Now().UTC()
	from := now.AddDate(0, -3, 0).Format("2006-01-02")
	to := now.AddDate(0, 0, 1).Format("2006-01-02")

	txns, err := store.ListTransactions(ctx, customerID, from, to)
	if err != nil {
		logger.Warn("financial context: transactions fetch failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		return nil
	}
	if len(txns) == 0 {
		return nil
	}

	// Limitar a 30 transações mais recentes (já vem desc por date)
	limit := 30
	if len(txns) < limit {
		limit = len(txns)
	}

	tc := &TransactionsContext{
		Recent: make([]TransactionSummary, limit),
		Count:  limit,
	}
	for i := 0; i < limit; i++ {
		tc.Recent[i] = TransactionSummary{
			Date:         txns[i].Date.Format("2006-01-02"),
			Amount:       txns[i].Amount,
			Type:         txns[i].Type,
			Category:     txns[i].Category,
			Description:  txns[i].Description,
			Counterparty: txns[i].Counterparty,
		}
	}

	return tc
}
