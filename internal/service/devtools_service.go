package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================
// Dev Tools
// ============================================================

// DevAddBalance adds the given amount to the customer's primary account balance.
func (s *BankingService) DevAddBalance(ctx context.Context, req *domain.DevAddBalanceRequest) (*domain.DevAddBalanceResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevAddBalance")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.Amount == 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "não pode ser zero"}
	}

	acct, err := s.store.UpdateAccountBalance(ctx, req.CustomerID, req.Amount)
	if err != nil {
		return nil, err
	}

	// Record the transaction for extrato/fatura
	now := time.Now()
	txType := "transfer_in"
	txDesc := fmt.Sprintf("DevTools — Crédito de saldo R$ %.2f", req.Amount)
	if req.Amount < 0 {
		txType = "transfer_out"
		txDesc = fmt.Sprintf("DevTools — Débito de saldo R$ %.2f", -req.Amount)
	}
	tx := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": req.CustomerID,
		"date":        now.Format(time.RFC3339),
		"description": txDesc,
		"amount":      req.Amount,
		"type":        txType,
		"category":    "devtools",
	}
	if txErr := s.store.InsertTransaction(ctx, tx); txErr != nil {
		s.logger.Error("DEV: failed to record balance transaction",
			zap.String("customer_id", req.CustomerID),
			zap.Error(txErr),
		)
		// Don't fail the whole operation — balance was already updated
	}

	s.logger.Info("DEV: balance adjusted",
		zap.String("customer_id", req.CustomerID),
		zap.Float64("amount", req.Amount),
		zap.Float64("new_balance", acct.Balance),
	)

	msg := fmt.Sprintf("R$ %.2f adicionados ao saldo", req.Amount)
	if req.Amount < 0 {
		msg = fmt.Sprintf("R$ %.2f debitados do saldo", -req.Amount)
	}
	return &domain.DevAddBalanceResponse{
		Success:    true,
		NewBalance: acct.Balance,
		Message:    msg,
	}, nil
}

// DevSetCreditLimit sets the credit limit of the customer's first credit card.
func (s *BankingService) DevSetCreditLimit(ctx context.Context, req *domain.DevSetCreditLimitRequest) (*domain.DevSetCreditLimitResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevSetCreditLimit")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.CreditLimit <= 0 {
		return nil, &domain.ErrValidation{Field: "creditLimit", Message: "deve ser positivo"}
	}

	err := s.store.UpdateCreditCardLimit(ctx, req.CustomerID, req.CreditLimit)
	if err != nil {
		return nil, err
	}

	// Record the transaction for extrato/fatura
	now := time.Now()
	tx := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": req.CustomerID,
		"date":        now.Format(time.RFC3339),
		"description": fmt.Sprintf("DevTools — Limite de crédito ajustado para R$ %.2f", req.CreditLimit),
		"amount":      0,
		"type":        "credit",
		"category":    "devtools",
	}
	if txErr := s.store.InsertTransaction(ctx, tx); txErr != nil {
		s.logger.Error("DEV: failed to record credit limit transaction",
			zap.String("customer_id", req.CustomerID),
			zap.Error(txErr),
		)
	}

	s.logger.Info("DEV: credit limit updated",
		zap.String("customer_id", req.CustomerID),
		zap.Float64("new_limit", req.CreditLimit),
	)

	return &domain.DevSetCreditLimitResponse{
		Success:  true,
		NewLimit: req.CreditLimit,
		Message:  fmt.Sprintf("Limite de crédito atualizado para R$ %.2f", req.CreditLimit),
	}, nil
}

// DevGenerateTransactions generates random transactions for testing.
func (s *BankingService) DevGenerateTransactions(ctx context.Context, req *domain.DevGenerateTransactionsRequest) (*domain.DevGenerateTransactionsResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevGenerateTransactions")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.Count <= 0 || req.Count > 100 {
		return nil, &domain.ErrValidation{Field: "count", Message: "deve ser entre 1 e 100"}
	}

	// Default months = 1, max 12
	months := req.Months
	// If period is set, it overrides months
	switch req.Period {
	case "current-month":
		months = 1
	case "last-12-months":
		months = 12
	}
	if months <= 0 {
		months = 1
	}
	if months > 12 {
		months = 12
	}
	daysSpan := months * 30 // approximate days to spread transactions across

	txTypes := []struct {
		Type     string
		IsDebit  bool
		Descs    []string
		Category string
	}{
		{"pix_sent", true, []string{"Pix enviado - Maria Silva", "Pix enviado - João LTDA", "Pix enviado - Ana Costa"}, "pix"},
		{"pix_received", false, []string{"Pix recebido - Tech Corp", "Pix recebido - Vendas Online", "Pix recebido - Cliente ABC"}, "recebimento"},
		{"debit_purchase", true, []string{"Supermercado Extra", "Posto Shell", "Farmácia São Paulo", "Restaurante Sabor"}, "compras"},
		{"credit_purchase", true, []string{"Amazon AWS", "Google Cloud", "Material Escritório", "Uber Business"}, "tecnologia"},
		{"transfer_in", false, []string{"TED recebida - Fornecedor A", "DOC recebido - Partner B", "Transferência recebida - Cliente"}, "recebimento"},
		{"transfer_out", true, []string{"TED enviada - Aluguel", "TED enviada - Fornecedor", "Transferência - Pagamento"}, "despesas"},
		{"bill_payment", true, []string{"Conta de luz", "Conta de telefone", "Internet Fibra", "IPTU"}, "contas"},
		{"credit", false, []string{"Crédito recebido", "Estorno - Compra duplicada", "Bonificação empresarial"}, "credito"},
		{"debit", true, []string{"Débito automático", "Tarifa bancária", "Cobrança serviço"}, "debito"},
	}

	generated := 0
	netImpact := 0.0
	totalIncome := 0.0
	totalExpenses := 0.0
	now := time.Now()

	for i := 0; i < req.Count; i++ {
		txInfo := txTypes[rand.Intn(len(txTypes))]
		desc := txInfo.Descs[rand.Intn(len(txInfo.Descs))]
		amount := float64(rand.Intn(490000)+1000) / 100.0 // R$ 10.00 to R$ 5000.00
		daysAgo := rand.Intn(daysSpan)
		txDate := now.AddDate(0, 0, -daysAgo)

		if txInfo.IsDebit {
			amount = -amount
		}

		tx := map[string]any{
			"id":          uuid.New().String(),
			"customer_id": req.CustomerID,
			"date":        txDate.Format(time.RFC3339),
			"description": desc,
			"amount":      amount,
			"type":        txInfo.Type,
			"category":    txInfo.Category,
		}

		if err := s.store.InsertTransaction(ctx, tx); err != nil {
			s.logger.Warn("DEV: failed to insert transaction", zap.Int("index", i), zap.Error(err))
			continue
		}
		generated++
		netImpact += amount // amount is already negative for debits
		if amount > 0 {
			totalIncome += amount
		} else {
			totalExpenses += -amount // store as positive value
		}
	}

	// Only update the account balance if explicitly requested via applyBalance flag.
	// By default, generated transactions are just for extrato/history and should NOT
	// change the real account balance — that was causing random balance fluctuations.
	if req.ApplyBalance && netImpact != 0 {
		if _, balErr := s.store.UpdateAccountBalance(ctx, req.CustomerID, netImpact); balErr != nil {
			s.logger.Error("DEV: failed to update balance after generating transactions",
				zap.String("customer_id", req.CustomerID),
				zap.Float64("net_impact", netImpact),
				zap.Error(balErr),
			)
		} else {
			s.logger.Info("DEV: balance adjusted after transaction generation",
				zap.Float64("net_impact", netImpact),
			)
		}
	}

	s.logger.Info("DEV: transactions generated",
		zap.String("customer_id", req.CustomerID),
		zap.Int("generated", generated),
	)

	return &domain.DevGenerateTransactionsResponse{
		Success:   true,
		Generated: generated,
		Income:    totalIncome,
		Expenses:  totalExpenses,
		NetImpact: netImpact,
		Message:   fmt.Sprintf("%d transações geradas com sucesso", generated),
	}, nil
}

// DevAddCardPurchase simulates credit card purchases for testing.
func (s *BankingService) DevAddCardPurchase(ctx context.Context, req *domain.DevAddCardPurchaseRequest) (*domain.DevAddCardPurchaseResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevAddCardPurchase")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.CardID == "" {
		return nil, &domain.ErrValidation{Field: "cardId", Message: "required"}
	}
	if req.Amount <= 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "deve ser positivo"}
	}
	if req.Mode != "today" && req.Mode != "random" {
		return nil, &domain.ErrValidation{Field: "mode", Message: "deve ser 'today' ou 'random'"}
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Mode == "today" {
		req.Count = 1
	}
	if req.Count > 50 {
		return nil, &domain.ErrValidation{Field: "count", Message: "máximo 50"}
	}

	// Verify card exists; auto-activate if pending
	card, err := s.store.GetCreditCard(ctx, req.CustomerID, req.CardID)
	if err != nil {
		return nil, err
	}
	if card.Status == "pending_activation" {
		if activateErr := s.store.UpdateCreditCardStatus(ctx, req.CardID, "active"); activateErr != nil {
			s.logger.Warn("DEV: failed to auto-activate card", zap.String("card_id", req.CardID), zap.Error(activateErr))
		} else {
			card.Status = "active"
			s.logger.Info("DEV: auto-activated pending card", zap.String("card_id", req.CardID))
		}
	}
	if card.Status != "active" {
		return nil, &domain.ErrValidation{Field: "cardId", Message: "cartão não está ativo"}
	}

	merchants := []struct {
		Name     string
		Category string
	}{
		{"Restaurante Sabor & Arte", "food"},
		{"Posto Shell BR-101", "fuel"},
		{"Amazon AWS", "technology"},
		{"Uber Business", "transport"},
		{"Netflix Assinatura", "subscription"},
		{"Google Cloud Platform", "technology"},
		{"iFood Corporativo", "food"},
		{"Kalunga Papelaria", "office_supplies"},
		{"99 Táxi Corporativo", "transport"},
		{"Adobe Creative Cloud", "subscription"},
		{"Hotel Ibis Business", "travel"},
		{"Seguro Porto PJ", "insurance"},
		{"Copel Energia", "utilities"},
		{"Google Ads", "marketing"},
		{"Contabilidade Express", "professional_services"},
		{"DAS Simples Nacional", "tax"},
		{"Limpeza & Manutenção", "maintenance"},
	}

	now := time.Now()
	generated := 0
	var totalAmount float64

	// Determine target month boundaries
	var monthStart, monthEnd time.Time
	if req.TargetMonth != "" {
		// Parse "YYYY-MM"
		parsed, parseErr := time.Parse("2006-01", req.TargetMonth)
		if parseErr != nil {
			return nil, &domain.ErrValidation{Field: "targetMonth", Message: "formato deve ser YYYY-MM"}
		}
		monthStart = parsed
		monthEnd = parsed.AddDate(0, 1, 0).Add(-time.Second) // last second of that month
	} else {
		// Default: current month
		monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		monthEnd = now
	}

	for i := 0; i < req.Count; i++ {
		m := merchants[rand.Intn(len(merchants))]

		var txDate time.Time
		if req.Mode == "today" && req.TargetMonth == "" {
			txDate = now
		} else {
			// Random date within the target month range
			dayRange := int(monthEnd.Sub(monthStart).Hours()/24) + 1
			if dayRange < 1 {
				dayRange = 1
			}
			randomDay := rand.Intn(dayRange)
			txDate = monthStart.AddDate(0, 0, randomDay)
			// Add random hour
			txDate = txDate.Add(time.Duration(rand.Intn(14)+8) * time.Hour)
			txDate = txDate.Add(time.Duration(rand.Intn(60)) * time.Minute)
		}

		tx := map[string]any{
			"id":                  uuid.New().String(),
			"card_id":             req.CardID,
			"customer_id":         req.CustomerID,
			"transaction_date":    txDate.Format(time.RFC3339),
			"amount":              req.Amount,
			"merchant_name":       m.Name,
			"category":            m.Category,
			"description":         fmt.Sprintf("Compra - %s", m.Name),
			"installments":        1,
			"current_installment": 1,
			"transaction_type":    "purchase",
			"status":              "confirmed",
		}

		if txErr := s.store.InsertCreditCardTransaction(ctx, tx); txErr != nil {
			s.logger.Warn("DEV: failed to insert card purchase", zap.Int("index", i), zap.Error(txErr))
			continue
		}
		generated++
		totalAmount += req.Amount
	}

	// Update card used_limit and available_limit
	if totalAmount > 0 {
		newUsed := card.UsedLimit + totalAmount
		newAvailable := card.CreditLimit - newUsed
		if newAvailable < 0 {
			newAvailable = 0
		}
		if err := s.store.UpdateCreditCardUsedLimit(ctx, req.CardID, newUsed, newAvailable); err != nil {
			s.logger.Error("DEV: failed to update card limits",
				zap.String("card_id", req.CardID),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("DEV: card purchases generated",
		zap.String("customer_id", req.CustomerID),
		zap.String("card_id", req.CardID),
		zap.Int("generated", generated),
		zap.Float64("total_amount", totalAmount),
	)

	return &domain.DevAddCardPurchaseResponse{
		Success:     true,
		Generated:   generated,
		TotalAmount: totalAmount,
		Message:     fmt.Sprintf("%d compras adicionadas ao cartão •••• %s", generated, card.CardNumberLast4),
	}, nil
}
