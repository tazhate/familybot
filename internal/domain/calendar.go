package domain

import "time"

// CalendarEvent represents a synced calendar event from Apple Calendar
type CalendarEvent struct {
	ID          int64
	UserID      int64
	CalDAVUID   string     // Unique ID from Apple Calendar
	Title       string     // Summary/Subject
	Description string     // Description
	Location    string     // Location
	StartTime   time.Time
	EndTime     time.Time
	AllDay      bool
	IsShared    bool       // Shared with partner
	SyncedAt    *time.Time // Last sync timestamp
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// FormatTime returns formatted time for display
func (e *CalendarEvent) FormatTime() string {
	if e.AllDay {
		return "Весь день"
	}
	if e.EndTime.IsZero() {
		return e.StartTime.Format("15:04")
	}
	return e.StartTime.Format("15:04") + "-" + e.EndTime.Format("15:04")
}

// FormatDate returns formatted date for display
func (e *CalendarEvent) FormatDate() string {
	return e.StartTime.Format("02.01")
}

// FormatDateTime returns formatted date and time
func (e *CalendarEvent) FormatDateTime() string {
	if e.AllDay {
		return e.StartTime.Format("02.01.2006") + " (весь день)"
	}
	return e.StartTime.Format("02.01.2006 15:04")
}

// IsToday returns true if event is today
func (e *CalendarEvent) IsToday() bool {
	now := time.Now()
	return e.StartTime.Year() == now.Year() &&
		e.StartTime.YearDay() == now.YearDay()
}

// IsTomorrow returns true if event is tomorrow
func (e *CalendarEvent) IsTomorrow() bool {
	tomorrow := time.Now().AddDate(0, 0, 1)
	return e.StartTime.Year() == tomorrow.Year() &&
		e.StartTime.YearDay() == tomorrow.YearDay()
}

// DaysUntil returns number of days until the event
func (e *CalendarEvent) DaysUntil() int {
	now := time.Now().Truncate(24 * time.Hour)
	eventDate := e.StartTime.Truncate(24 * time.Hour)
	return int(eventDate.Sub(now).Hours() / 24)
}

// LocationEmoji returns location emoji if location is set
func (e *CalendarEvent) LocationEmoji() string {
	if e.Location != "" {
		return " 📍"
	}
	return ""
}
