package service

import (
	"context"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
)

// ============================================================
// Spending Analytics
// ============================================================

func (s *BankingService) GetSpendingSummary(ctx context.Context, customerID, periodType string) (*domain.SpendingSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetSpendingSummary")
	defer span.End()

	return s.store.GetSpendingSummary(ctx, customerID, periodType)
}

func (s *BankingService) GetCategoryBreakdown(ctx context.Context, customerID, periodType string) (map[string]domain.CatSum, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCategoryBreakdown")
	defer span.End()

	summary, err := s.store.GetSpendingSummary(ctx, customerID, periodType)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return map[string]domain.CatSum{}, nil
	}
	return summary.CategoryBreakdown, nil
}

func (s *BankingService) ListBudgets(ctx context.Context, customerID string) ([]domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListBudgets")
	defer span.End()

	return s.store.ListBudgets(ctx, customerID)
}

func (s *BankingService) CreateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateBudget")
	defer span.End()

	if budget.Category == "" {
		return nil, &domain.ErrValidation{Field: "category", Message: "required"}
	}
	if budget.MonthlyLimit <= 0 {
		return nil, &domain.ErrValidation{Field: "monthly_limit", Message: "must be positive"}
	}
	if budget.AlertThresholdPct == 0 {
		budget.AlertThresholdPct = 80.0
	}
	budget.IsActive = true

	return s.store.CreateBudget(ctx, budget)
}

func (s *BankingService) UpdateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.UpdateBudget")
	defer span.End()

	return s.store.UpdateBudget(ctx, budget)
}

// ============================================================
// Favorites
// ============================================================

func (s *BankingService) ListFavorites(ctx context.Context, customerID string) ([]domain.Favorite, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListFavorites")
	defer span.End()

	return s.store.ListFavorites(ctx, customerID)
}

func (s *BankingService) CreateFavorite(ctx context.Context, fav *domain.Favorite) (*domain.Favorite, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateFavorite")
	defer span.End()

	if fav.Nickname == "" {
		return nil, &domain.ErrValidation{Field: "nickname", Message: "required"}
	}
	if fav.RecipientName == "" {
		return nil, &domain.ErrValidation{Field: "recipient_name", Message: "required"}
	}

	return s.store.CreateFavorite(ctx, fav)
}

func (s *BankingService) DeleteFavorite(ctx context.Context, customerID, favoriteID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeleteFavorite")
	defer span.End()

	return s.store.DeleteFavorite(ctx, customerID, favoriteID)
}

// ============================================================
// Transaction Limits
// ============================================================

func (s *BankingService) ListLimits(ctx context.Context, customerID string) ([]domain.TransactionLimit, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListLimits")
	defer span.End()

	return s.store.ListTransactionLimits(ctx, customerID)
}

func (s *BankingService) UpdateLimit(ctx context.Context, limit *domain.TransactionLimit) (*domain.TransactionLimit, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.UpdateLimit")
	defer span.End()

	return s.store.UpdateTransactionLimit(ctx, limit)
}

// ============================================================
// Notifications
// ============================================================

func (s *BankingService) ListNotifications(ctx context.Context, customerID string, unreadOnly bool, page, pageSize int) ([]domain.Notification, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListNotifications")
	defer span.End()

	return s.store.ListNotifications(ctx, customerID, unreadOnly, page, pageSize)
}

func (s *BankingService) MarkNotificationRead(ctx context.Context, notifID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.MarkNotificationRead")
	defer span.End()

	return s.store.MarkNotificationRead(ctx, notifID)
}

// ============================================================
// Financial Summary (aggregated view for the frontend spec)
// ============================================================

func (s *BankingService) GetFinancialSummary(ctx context.Context, customerID, period string) (*domain.FinancialSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetFinancialSummary")
	defer span.End()

	// Get account balance
	account, err := s.store.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		s.logger.Warn("no account for financial summary", zap.String("customer_id", customerID), zap.Error(err))
		account = &domain.Account{}
	}

	// Determine period label and dates
	now := time.Now()
	periodLabel := "Últimos 30 dias"
	periodDays := 30
	switch period {
	case "7d":
		periodLabel = "Últimos 7 dias"
		periodDays = 7
	case "90d", "3months":
		periodLabel = "Últimos 3 meses"
		periodDays = 90
	case "6months":
		periodLabel = "Últimos 6 meses"
		periodDays = 180
	case "12m", "1year":
		periodLabel = "Últimos 12 meses"
		periodDays = 365
	case "1month", "30d":
		periodLabel = "Últimos 30 dias"
		periodDays = 30
	}
	fromDate := now.AddDate(0, 0, -periodDays).Format("2006-01-02")
	toDate := now.AddDate(0, 0, 1).Format("2006-01-02") // next day so we include all of today

	// Fetch actual transactions from customer_transactions
	txns, txErr := s.store.ListTransactions(ctx, customerID, fromDate, toDate)
	if txErr != nil {
		s.logger.Warn("could not list transactions for financial summary", zap.Error(txErr))
		txns = nil
	}

	// Compute income, expenses, and category breakdown from real transactions
	var totalIncome, totalExpenses float64
	categoryMap := make(map[string]struct {
		Total float64
		Count int
	})

	// Monthly breakdown for trend
	monthlyIncome := make(map[string]float64)
	monthlyExpenses := make(map[string]float64)

	for _, tx := range txns {
		monthKey := tx.Date.Format("2006-01")
		if tx.Amount >= 0 {
			totalIncome += tx.Amount
			monthlyIncome[monthKey] += tx.Amount
		} else {
			totalExpenses += -tx.Amount // store as positive for display
			monthlyExpenses[monthKey] += -tx.Amount
		}
		if tx.Category != "" {
			entry := categoryMap[tx.Category]
			entry.Total += -tx.Amount // positive value for expense categories
			if tx.Amount < 0 {
				entry.Count++
			}
			categoryMap[tx.Category] = entry
		}
	}

	// Build top categories
	topCategories := make([]domain.TopCategory, 0)
	for cat, info := range categoryMap {
		if info.Total <= 0 {
			continue // skip income categories
		}
		pct := float64(0)
		if totalExpenses > 0 {
			pct = (info.Total / totalExpenses) * 100
		}
		topCategories = append(topCategories, domain.TopCategory{
			Category:         cat,
			Amount:           info.Total,
			Percentage:       pct,
			TransactionCount: info.Count,
			Trend:            "stable",
		})
	}

	// Build monthly trend
	monthlyTrend := make([]domain.MonthlyTrend, 0)
	monthSet := make(map[string]bool)
	for m := range monthlyIncome {
		monthSet[m] = true
	}
	for m := range monthlyExpenses {
		monthSet[m] = true
	}
	for m := range monthSet {
		inc := monthlyIncome[m]
		exp := monthlyExpenses[m]
		monthlyTrend = append(monthlyTrend, domain.MonthlyTrend{
			Month:    m,
			Income:   inc,
			Expenses: exp,
			Balance:  inc - exp,
		})
	}

	// Sort monthly trend by month ascending
	for i := 0; i < len(monthlyTrend); i++ {
		for j := i + 1; j < len(monthlyTrend); j++ {
			if monthlyTrend[i].Month > monthlyTrend[j].Month {
				monthlyTrend[i], monthlyTrend[j] = monthlyTrend[j], monthlyTrend[i]
			}
		}
	}

	netCashFlow := totalIncome - totalExpenses
	avgDaily := float64(0)
	if periodDays > 0 && totalExpenses > 0 {
		avgDaily = totalExpenses / float64(periodDays)
	}

	return &domain.FinancialSummary{
		CustomerID: customerID,
		Period: &domain.FinancialPeriod{
			From:  fromDate,
			To:    toDate,
			Label: periodLabel,
		},
		Balance: &domain.BalanceSummary{
			Current:   account.Balance,
			Available: account.AvailableBalance,
			Blocked:   account.Balance - account.AvailableBalance,
			Invested:  0,
		},
		CashFlow: &domain.CashFlowSummary{
			TotalIncome:              totalIncome,
			TotalExpenses:            totalExpenses,
			NetCashFlow:              netCashFlow,
			ComparedToPreviousPeriod: 0,
		},
		Spending: &domain.SpendingDetail{
			TotalSpent:               totalExpenses,
			AverageDaily:             avgDaily,
			ComparedToPreviousPeriod: 0,
		},
		TopCategories: topCategories,
		MonthlyTrend:  monthlyTrend,
	}, nil
}

// GetTransactionSummary computes an aggregated summary of customer transactions.
// Balance reflects the real account balance, not just sum of transactions.
func (s *BankingService) GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetTransactionSummary")
	defer span.End()

	summary, err := s.store.GetTransactionSummary(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Override balance with real account balance
	account, acctErr := s.store.GetPrimaryAccount(ctx, customerID)
	if acctErr == nil && account != nil {
		summary.Balance = account.Balance
	}

	return summary, nil
}
