package debtmanager

import "time"

// Debt represents a debt from debt-manager
type Debt struct {
	ID             uint      `json:"id"`
	Name           string    `json:"name"`
	TotalAmount    float64   `json:"total_amount"`
	CurrentAmount  float64   `json:"current_amount"`
	MonthlyPayment float64   `json:"monthly_payment"`
	PaymentDay     int       `json:"payment_day"`
	InterestRate   float64   `json:"interest_rate"`
	Category       string    `json:"category"`
	Priority       int       `json:"priority"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Income represents an income source from debt-manager
type Income struct {
	ID          uint      `json:"id"`
	Source      string    `json:"source"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	AmountRUB   float64   `json:"amount_rub"`
	PaymentDay  int       `json:"payment_day"`
	TaxRate     float64   `json:"tax_rate"`
	IsRecurring bool      `json:"is_recurring"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Dashboard represents the dashboard summary
type Dashboard struct {
	TotalDebt       float64 `json:"total_debt"`
	TotalPaid       float64 `json:"total_paid"`
	MonthlyPayment  float64 `json:"monthly_payment"`
	MonthlyIncome   float64 `json:"monthly_income"`
	DebtCount       int     `json:"debt_count"`
	ProgressPercent float64 `json:"progress_percent"`
}

// Payment represents a payment record
type Payment struct {
	ID          uint      `json:"id"`
	DebtID      uint      `json:"debt_id"`
	Amount      float64   `json:"amount"`
	PaymentDate time.Time `json:"payment_date"`
	Status      string    `json:"status"` // pending, paid, skipped
	Notes       string    `json:"notes"`
}

// PaymentStatus represents payment status for tracking
type PaymentStatus struct {
	ID          uint      `json:"id"`
	PaymentType string    `json:"payment_type"` // debt, income, extra
	PaymentID   uint      `json:"payment_id"`
	PaymentDate time.Time `json:"payment_date"`
	Status      string    `json:"status"` // scheduled, paid, skipped
}

// CategoryEmoji returns emoji for debt category
func CategoryEmoji(category string) string {
	switch category {
	case "bank":
		return "🏦"
	case "auto":
		return "🚗"
	case "credit_card":
		return "💳"
	case "mfo":
		return "⚠️"
	case "tax":
		return "📋"
	case "alimony":
		return "👶"
	case "personal":
		return "👤"
	default:
		return "💰"
	}
}
