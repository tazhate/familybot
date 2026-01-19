package domain

import "time"

type ReminderType string

const (
	ReminderDaily      ReminderType = "daily"
	ReminderWeekly     ReminderType = "weekly"
	ReminderMonthly    ReminderType = "monthly"
	ReminderMonthWeek  ReminderType = "month_week" // 2-я пятница месяца
	ReminderYearly     ReminderType = "yearly"
	ReminderFloating   ReminderType = "floating"   // требует подтверждения
)

type Reminder struct {
	ID        int64
	UserID    int64
	Title     string
	Type      ReminderType
	Schedule  string // cron expression
	Params    string // JSON с доп. параметрами
	IsActive  bool
	LastSent  *time.Time
	NextRun   *time.Time
	CreatedAt time.Time
}

type ReminderParams struct {
	Time       string `json:"time,omitempty"`        // "09:00"
	DayOfWeek  int    `json:"day_of_week,omitempty"` // 0-6 (Sun-Sat)
	DayOfMonth int    `json:"day_of_month,omitempty"`
	WeekOfMonth int   `json:"week_of_month,omitempty"` // 1-5
	Month      int    `json:"month,omitempty"`         // 1-12
	Day        int    `json:"day,omitempty"`           // 1-31
}
