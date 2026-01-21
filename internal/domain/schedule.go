package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Weekday represents a day of the week (0 = Sunday, 1 = Monday, ...)
type Weekday int

const (
	WeekdaySunday    Weekday = 0
	WeekdayMonday    Weekday = 1
	WeekdayTuesday   Weekday = 2
	WeekdayWednesday Weekday = 3
	WeekdayThursday  Weekday = 4
	WeekdayFriday    Weekday = 5
	WeekdaySaturday  Weekday = 6
)

// WeeklyEvent represents a recurring weekly event
type WeeklyEvent struct {
	ID             int64
	UserID         int64
	DayOfWeek      Weekday // 0-6 (Sunday-Saturday)
	TimeStart      string  // "HH:MM"
	TimeEnd        string  // "HH:MM" (optional)
	Title          string
	PersonID       *int64  // Optional link to person
	ReminderBefore int     // Minutes before to remind (0 = no reminder)
	IsFloating     bool    // Плавающее событие (выбор дня на неделе)
	FloatingDays   string  // Дни для плавающего события, напр "6,0" (Сб, Вс)
	ConfirmedDay   *int    // Подтверждённый день на эту неделю (nil = не выбран)
	ConfirmedWeek  int     // ISO неделя года когда был подтверждён день
	IsShared       bool    // Общее событие (видно всей семье)
	CreatedAt      time.Time
}

// IsConfirmedThisWeek checks if floating event has confirmed day for current week
func (e *WeeklyEvent) IsConfirmedThisWeek() bool {
	if !e.IsFloating || e.ConfirmedDay == nil {
		return false
	}
	_, week := time.Now().ISOWeek()
	return e.ConfirmedWeek == week
}

// GetFloatingDays returns list of valid weekdays for floating event
func (e *WeeklyEvent) GetFloatingDays() []Weekday {
	if e.FloatingDays == "" {
		return nil
	}
	var days []Weekday
	for _, s := range strings.Split(e.FloatingDays, ",") {
		if d, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			days = append(days, Weekday(d))
		}
	}
	return days
}

// SetFloatingDays sets valid weekdays for floating event
func (e *WeeklyEvent) SetFloatingDays(days []Weekday) {
	var parts []string
	for _, d := range days {
		parts = append(parts, strconv.Itoa(int(d)))
	}
	e.FloatingDays = strings.Join(parts, ",")
}

// DayName returns Russian name for the day
func (e *WeeklyEvent) DayName() string {
	return WeekdayName(e.DayOfWeek)
}

// DayNameShort returns short Russian name for the day
func (e *WeeklyEvent) DayNameShort() string {
	return WeekdayNameShort(e.DayOfWeek)
}

// TimeRange returns formatted time range
func (e *WeeklyEvent) TimeRange() string {
	if e.TimeEnd != "" {
		return e.TimeStart + "-" + e.TimeEnd
	}
	return e.TimeStart
}

// WeekdayName returns Russian name for the weekday
func WeekdayName(d Weekday) string {
	names := []string{"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}
	if d >= 0 && int(d) < len(names) {
		return names[d]
	}
	return ""
}

// WeekdayNameShort returns short Russian name for the weekday
func WeekdayNameShort(d Weekday) string {
	names := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}
	if d >= 0 && int(d) < len(names) {
		return names[d]
	}
	return ""
}

// WeekdayEmoji returns emoji for the weekday
func WeekdayEmoji(d Weekday) string {
	emojis := []string{"🌅", "📅", "📅", "📅", "📅", "🎉", "🌴"}
	if d >= 0 && int(d) < len(emojis) {
		return emojis[d]
	}
	return "📅"
}

// ParseWeekday parses Russian weekday name
func ParseWeekday(s string) (Weekday, bool) {
	mapping := map[string]Weekday{
		"пн": WeekdayMonday, "понедельник": WeekdayMonday,
		"вт": WeekdayTuesday, "вторник": WeekdayTuesday,
		"ср": WeekdayWednesday, "среда": WeekdayWednesday,
		"чт": WeekdayThursday, "четверг": WeekdayThursday,
		"пт": WeekdayFriday, "пятница": WeekdayFriday,
		"сб": WeekdaySaturday, "суббота": WeekdaySaturday,
		"вс": WeekdaySunday, "воскресенье": WeekdaySunday,
	}

	if d, ok := mapping[s]; ok {
		return d, true
	}
	return 0, false
}

// ParseWeekdayShort parses short Russian weekday name and returns time.Weekday
func ParseWeekdayShort(s string) (time.Weekday, error) {
	mapping := map[string]time.Weekday{
		"пн": time.Monday, "понедельник": time.Monday,
		"вт": time.Tuesday, "вторник": time.Tuesday,
		"ср": time.Wednesday, "среда": time.Wednesday,
		"чт": time.Thursday, "четверг": time.Thursday,
		"пт": time.Friday, "пятница": time.Friday,
		"сб": time.Saturday, "суббота": time.Saturday,
		"вс": time.Sunday, "воскресенье": time.Sunday,
	}

	if d, ok := mapping[strings.ToLower(s)]; ok {
		return d, nil
	}
	return time.Sunday, fmt.Errorf("unknown weekday: %s", s)
}
