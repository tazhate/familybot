package domain

import "time"

type Auto struct {
	ID               int64
	UserID           int64
	Name             string // Название (например "Ford Raptor")
	Year             int    // Год выпуска
	InsuranceUntil   *time.Time
	MaintenanceUntil *time.Time
	Notes            string
	CreatedAt        time.Time
}

// DaysUntilInsurance returns days until insurance expires (negative if expired)
func (a *Auto) DaysUntilInsurance() int {
	if a.InsuranceUntil == nil {
		return 999
	}
	return int(time.Until(*a.InsuranceUntil).Hours() / 24)
}

// DaysUntilMaintenance returns days until maintenance is due (negative if overdue)
func (a *Auto) DaysUntilMaintenance() int {
	if a.MaintenanceUntil == nil {
		return 999
	}
	return int(time.Until(*a.MaintenanceUntil).Hours() / 24)
}

// InsuranceStatus returns emoji + text for insurance status
func (a *Auto) InsuranceStatus() string {
	if a.InsuranceUntil == nil {
		return "❓ не указано"
	}
	days := a.DaysUntilInsurance()
	switch {
	case days < 0:
		return "🔴 просрочено"
	case days <= 7:
		return "🟠 скоро"
	case days <= 30:
		return "🟡 через месяц"
	default:
		return "🟢 ok"
	}
}

// MaintenanceStatus returns emoji + text for maintenance status
func (a *Auto) MaintenanceStatus() string {
	if a.MaintenanceUntil == nil {
		return "❓ не указано"
	}
	days := a.DaysUntilMaintenance()
	switch {
	case days < 0:
		return "🔴 просрочено"
	case days <= 7:
		return "🟠 скоро"
	case days <= 30:
		return "🟡 через месяц"
	default:
		return "🟢 ok"
	}
}
