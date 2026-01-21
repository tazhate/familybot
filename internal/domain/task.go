package domain

import (
	"strings"
	"time"
)

type Priority string

const (
	PriorityUrgent  Priority = "urgent"
	PriorityWeek    Priority = "week"
	PrioritySomeday Priority = "someday"
)

type RepeatType string

const (
	RepeatNone       RepeatType = ""
	RepeatDaily      RepeatType = "daily"       // Каждый день
	RepeatWeekdays   RepeatType = "weekdays"    // Пн-Пт
	RepeatWeekly     RepeatType = "weekly"      // Раз в неделю
	RepeatMonthly    RepeatType = "monthly"     // Раз в месяц (тот же день)
	RepeatMonthlyNth RepeatType = "monthly_nth" // N-й день недели месяца (напр. 2-я пятница)
)

type Task struct {
	ID          int64
	UserID      int64
	ChatID      int64  // Контекст чата (личный или групповой)
	AssignedTo  *int64
	PersonID    *int64 // Связь с Person (для кого задача)
	Title       string
	Description string
	Priority    Priority
	IsShared    bool
	DueDate     *time.Time
	DoneAt      *time.Time
	CreatedAt   time.Time

	// Повторные напоминания
	ReminderCount  int        // Сколько раз напоминали
	LastRemindedAt *time.Time // Когда последний раз напоминали
	SnoozeUntil    *time.Time // Отложено до этого времени

	// Повторяющиеся задачи
	RepeatType    RepeatType // Тип повторения
	RepeatTime    string     // Время напоминания "HH:MM"
	RepeatWeekNum int        // Номер недели месяца (1-4) для monthly_nth
}

func (t *Task) IsDone() bool {
	return t.DoneAt != nil
}

func (t *Task) IsRepeating() bool {
	return t.RepeatType != RepeatNone && t.RepeatType != ""
}

func (t *Task) PriorityEmoji() string {
	switch t.Priority {
	case PriorityUrgent:
		return "🔴"
	case PriorityWeek:
		return "🟡"
	case PrioritySomeday:
		return "🟢"
	default:
		return "⚪"
	}
}

func (t *Task) RepeatEmoji() string {
	if !t.IsRepeating() {
		return ""
	}
	return "🔁"
}

// NextOccurrence calculates the next due date for a repeating task
func (t *Task) NextOccurrence(from time.Time) *time.Time {
	if !t.IsRepeating() {
		return nil
	}

	var next time.Time

	switch t.RepeatType {
	case RepeatDaily:
		next = from.AddDate(0, 0, 1)

	case RepeatWeekdays:
		next = from.AddDate(0, 0, 1)
		// Skip weekends
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}

	case RepeatWeekly:
		next = from.AddDate(0, 0, 7)

	case RepeatMonthly:
		next = from.AddDate(0, 1, 0)

	case RepeatMonthlyNth:
		// N-й день недели месяца (напр. 2-я пятница)
		weekday := from.Weekday()
		weekNum := t.RepeatWeekNum
		if weekNum < 1 || weekNum > 4 {
			weekNum = 1
		}
		// Ищем в следующем месяце
		nextMonth := from.AddDate(0, 1, 0)
		next = NthWeekdayOfMonth(nextMonth.Year(), nextMonth.Month(), weekday, weekNum)

	default:
		return nil
	}

	// Set time if specified
	if t.RepeatTime != "" {
		hour, min := 9, 0 // default
		if len(t.RepeatTime) >= 5 {
			_, _ = time.Parse("15:04", t.RepeatTime)
			var h, m int
			if n, _ := parseHHMM(t.RepeatTime); n > 0 {
				h, m = n/100, n%100
				hour, min = h, m
			}
		}
		next = time.Date(next.Year(), next.Month(), next.Day(), hour, min, 0, 0, next.Location())
	}

	return &next
}

// NthWeekdayOfMonth returns the Nth weekday of a given month
// weekday: 0=Sunday, 1=Monday, ..., 5=Friday, 6=Saturday
// n: 1=first, 2=second, 3=third, 4=fourth
func NthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	// Start from the first day of the month
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)

	// Find the first occurrence of the weekday
	daysUntil := int(weekday) - int(first.Weekday())
	if daysUntil < 0 {
		daysUntil += 7
	}
	firstOccurrence := first.AddDate(0, 0, daysUntil)

	// Add (n-1) weeks to get the Nth occurrence
	return firstOccurrence.AddDate(0, 0, (n-1)*7)
}

// WeekOfMonth returns which week of the month a date falls in (1-4)
func WeekOfMonth(t time.Time) int {
	day := t.Day()
	return (day-1)/7 + 1
}

// parseHHMM parses "HH:MM" and returns HHMM as int
func parseHHMM(s string) (int, error) {
	if len(s) < 5 {
		return 0, nil
	}
	var h, m int
	for i := 0; i < 2; i++ {
		h = h*10 + int(s[i]-'0')
	}
	for i := 3; i < 5; i++ {
		m = m*10 + int(s[i]-'0')
	}
	return h*100 + m, nil
}

// TaskReminder represents a reminder for a task (before due_date)
type TaskReminder struct {
	ID           int64
	TaskID       int64
	RemindBefore int        // Minutes before due_date
	SentAt       *time.Time // When reminder was sent (nil = not sent)
}

// Preset reminder intervals in minutes
const (
	RemindWeek     = 7 * 24 * 60 // 10080 min
	RemindDay      = 24 * 60     // 1440 min
	Remind3Hours   = 3 * 60      // 180 min
	RemindHour     = 60          // 60 min
	Remind30Min    = 30          // 30 min
)

// RemindBeforeLabel returns human-readable label for remind_before value
func RemindBeforeLabel(minutes int) string {
	switch minutes {
	case RemindWeek:
		return "за неделю"
	case RemindDay:
		return "за день"
	case Remind3Hours:
		return "за 3 часа"
	case RemindHour:
		return "за час"
	case Remind30Min:
		return "за 30 мин"
	default:
		if minutes >= 1440 {
			return "за " + string(rune('0'+minutes/1440)) + " дн."
		}
		if minutes >= 60 {
			return "за " + string(rune('0'+minutes/60)) + " ч."
		}
		return "за " + string(rune('0'+minutes)) + " мин."
	}
}

// ParseRemindInterval parses Russian interval like "1д", "3ч", "30м", "неделя"
func ParseRemindInterval(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))

	switch s {
	case "неделя", "нед", "1н":
		return RemindWeek, true
	case "день", "1д", "1d":
		return RemindDay, true
	case "3ч", "3h":
		return Remind3Hours, true
	case "час", "1ч", "1h":
		return RemindHour, true
	case "30м", "30m", "30мин":
		return Remind30Min, true
	}

	// Try parsing Xд, Xч, Xм format
	if len(s) >= 2 {
		numStr := s[:len(s)-2]
		suffix := s[len(s)-2:]

		// Handle single-byte suffixes
		if len(s) >= 1 {
			lastByte := s[len(s)-1]
			if lastByte >= '0' && lastByte <= '9' {
				return 0, false
			}
		}

		var num int
		for _, c := range numStr {
			if c >= '0' && c <= '9' {
				num = num*10 + int(c-'0')
			}
		}

		if num > 0 {
			switch {
			case strings.HasPrefix(suffix, "д") || suffix == "d":
				return num * 1440, true
			case strings.HasPrefix(suffix, "ч") || suffix == "h":
				return num * 60, true
			case strings.HasPrefix(suffix, "м") || suffix == "m":
				return num, true
			}
		}
	}

	return 0, false
}
