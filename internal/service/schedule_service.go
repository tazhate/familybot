package service

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type ScheduleService struct {
	storage *storage.Storage
}

func NewScheduleService(s *storage.Storage) *ScheduleService {
	return &ScheduleService{storage: s}
}

// Create creates a new weekly event
func (s *ScheduleService) Create(userID int64, dayOfWeek domain.Weekday, timeStart, timeEnd, title string, reminderBefore int) (*domain.WeeklyEvent, error) {
	if title == "" {
		return nil, errors.New("название не может быть пустым")
	}

	event := &domain.WeeklyEvent{
		UserID:         userID,
		DayOfWeek:      dayOfWeek,
		TimeStart:      timeStart,
		TimeEnd:        timeEnd,
		Title:          title,
		ReminderBefore: reminderBefore,
	}

	if err := s.storage.CreateWeeklyEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

// List returns all weekly events for a user (including shared if requested)
func (s *ScheduleService) List(userID int64, includeShared bool) ([]*domain.WeeklyEvent, error) {
	return s.storage.ListWeeklyEventsByUser(userID, includeShared)
}

// ListForDay returns events for a specific day (including shared)
func (s *ScheduleService) ListForDay(userID int64, day domain.Weekday, includeShared bool) ([]*domain.WeeklyEvent, error) {
	return s.storage.ListWeeklyEventsByDay(userID, day, includeShared)
}

// ListForToday returns events for today (including shared)
func (s *ScheduleService) ListForToday(userID int64, includeShared bool) ([]*domain.WeeklyEvent, error) {
	today := domain.Weekday(time.Now().Weekday())
	return s.ListForDay(userID, today, includeShared)
}

// SetShared updates the is_shared flag for an event
func (s *ScheduleService) SetShared(eventID int64, userID int64, isShared bool) error {
	event, err := s.storage.GetWeeklyEvent(eventID)
	if err != nil {
		return err
	}
	if event == nil {
		return errors.New("событие не найдено")
	}
	if event.UserID != userID {
		return errors.New("нет доступа")
	}
	return s.storage.UpdateWeeklyEventShared(eventID, isShared)
}

// Delete deletes an event
func (s *ScheduleService) Delete(id int64, userID int64) error {
	event, err := s.storage.GetWeeklyEvent(id)
	if err != nil {
		return err
	}
	if event == nil {
		return errors.New("событие не найдено")
	}
	if event.UserID != userID {
		return errors.New("нет доступа")
	}
	return s.storage.DeleteWeeklyEvent(id)
}

// CreateFloating creates a new floating weekly event
func (s *ScheduleService) CreateFloating(userID int64, days []domain.Weekday, timeStart, timeEnd, title string) (*domain.WeeklyEvent, error) {
	if title == "" {
		return nil, errors.New("название не может быть пустым")
	}
	if len(days) < 2 {
		return nil, errors.New("нужно указать минимум 2 дня")
	}

	event := &domain.WeeklyEvent{
		UserID:         userID,
		DayOfWeek:      days[0], // Первый день по умолчанию
		TimeStart:      timeStart,
		TimeEnd:        timeEnd,
		Title:          title,
		IsFloating:     true,
		ReminderBefore: 30, // Дефолтное напоминание за 30 минут для плавающих событий
	}
	event.SetFloatingDays(days)

	if err := s.storage.CreateWeeklyEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

// ConfirmFloatingDay confirms the day for a floating event this week
func (s *ScheduleService) ConfirmFloatingDay(eventID int64, userID int64, day domain.Weekday) error {
	event, err := s.storage.GetWeeklyEvent(eventID)
	if err != nil {
		return err
	}
	if event == nil {
		return errors.New("событие не найдено")
	}
	if event.UserID != userID {
		return errors.New("нет доступа")
	}
	if !event.IsFloating {
		return errors.New("это не плавающее событие")
	}

	// Validate the day is in the allowed list
	validDays := event.GetFloatingDays()
	isValid := false
	for _, d := range validDays {
		if d == day {
			isValid = true
			break
		}
	}
	if !isValid {
		return errors.New("недопустимый день для этого события")
	}

	_, week := time.Now().ISOWeek()
	dayInt := int(day)
	return s.storage.UpdateWeeklyEventConfirmedDay(eventID, &dayInt, week)
}

// ListFloating returns all floating events for a user
func (s *ScheduleService) ListFloating(userID int64) ([]*domain.WeeklyEvent, error) {
	return s.storage.ListFloatingEvents(userID)
}

// Get returns a specific event
func (s *ScheduleService) Get(id int64) (*domain.WeeklyEvent, error) {
	return s.storage.GetWeeklyEvent(id)
}

// ParseFloatingArgs parses "/addfloating Сб,Вс 10:00 Лука" format
func (s *ScheduleService) ParseFloatingArgs(args string) (days []domain.Weekday, timeStart, timeEnd, title string, err error) {
	parts := strings.Fields(args)
	if len(parts) < 3 {
		err = errors.New("формат: /addfloating Сб,Вс 10:00 Название")
		return
	}

	// Parse days (comma-separated)
	dayStrs := strings.Split(parts[0], ",")
	for _, ds := range dayStrs {
		day, ok := domain.ParseWeekday(strings.ToLower(strings.TrimSpace(ds)))
		if !ok {
			err = fmt.Errorf("неверный день: %s", ds)
			return
		}
		days = append(days, day)
	}

	if len(days) < 2 {
		err = errors.New("укажите минимум 2 дня через запятую")
		return
	}

	// Parse time
	timeStr := parts[1]
	if strings.Contains(timeStr, "-") {
		timeParts := strings.Split(timeStr, "-")
		timeStart = timeParts[0]
		if len(timeParts) > 1 {
			timeEnd = timeParts[1]
		}
	} else {
		timeStart = timeStr
	}

	// Validate time format
	timeRe := regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	if !timeRe.MatchString(timeStart) {
		err = errors.New("неверный формат времени (ЧЧ:ММ)")
		return
	}

	// Rest is title
	title = strings.Join(parts[2:], " ")
	return
}

// ParseAddArgs parses "/addweekly Пн 17:30 Федя спорт" or "/addweekly Пн 17:30 !15 Федя спорт" format
func (s *ScheduleService) ParseAddArgs(args string) (dayOfWeek domain.Weekday, timeStart, timeEnd, title string, reminderBefore int, err error) {
	parts := strings.Fields(args)
	if len(parts) < 3 {
		err = errors.New("формат: /addweekly Пн 17:30 Название")
		return
	}

	// Parse day of week
	day, ok := domain.ParseWeekday(strings.ToLower(parts[0]))
	if !ok {
		err = errors.New("неверный день недели (Пн, Вт, Ср, Чт, Пт, Сб, Вс)")
		return
	}
	dayOfWeek = day

	// Parse time (could be "17:30" or "16:00-20:00")
	timeStr := parts[1]
	if strings.Contains(timeStr, "-") {
		timeParts := strings.Split(timeStr, "-")
		timeStart = timeParts[0]
		if len(timeParts) > 1 {
			timeEnd = timeParts[1]
		}
	} else {
		timeStart = timeStr
	}

	// Validate time format
	timeRe := regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	if !timeRe.MatchString(timeStart) {
		err = errors.New("неверный формат времени (ЧЧ:ММ)")
		return
	}

	// Check for reminder prefix !N (e.g., !15 for 15 minutes before)
	titleParts := parts[2:]
	if len(titleParts) > 0 && strings.HasPrefix(titleParts[0], "!") {
		reminderStr := strings.TrimPrefix(titleParts[0], "!")
		if mins, parseErr := strconv.Atoi(reminderStr); parseErr == nil && mins > 0 {
			reminderBefore = mins
			titleParts = titleParts[1:]
		}
	}

	// Rest is title
	title = strings.Join(titleParts, " ")
	if title == "" {
		err = errors.New("название не может быть пустым")
	}
	return
}

// FormatWeekSchedule formats the weekly schedule
func (s *ScheduleService) FormatWeekSchedule(events []*domain.WeeklyEvent) string {
	return s.formatWeekScheduleInternal(events, false)
}

// FormatWeekScheduleWithIDs formats the weekly schedule with event IDs
func (s *ScheduleService) FormatWeekScheduleWithIDs(events []*domain.WeeklyEvent) string {
	return s.formatWeekScheduleInternal(events, true)
}

func (s *ScheduleService) formatWeekScheduleInternal(events []*domain.WeeklyEvent, showIDs bool) string {
	if len(events) == 0 {
		return "Расписание пусто"
	}

	// Separate floating and regular events
	var floatingEvents []*domain.WeeklyEvent
	byDay := make(map[domain.Weekday][]*domain.WeeklyEvent)

	for _, e := range events {
		if e.IsFloating {
			floatingEvents = append(floatingEvents, e)
			// If confirmed this week, also show in the confirmed day
			if e.IsConfirmedThisWeek() && e.ConfirmedDay != nil {
				byDay[domain.Weekday(*e.ConfirmedDay)] = append(byDay[domain.Weekday(*e.ConfirmedDay)], e)
			}
		} else {
			byDay[e.DayOfWeek] = append(byDay[e.DayOfWeek], e)
		}
	}

	var sb strings.Builder

	// Show floating events that need confirmation
	unconfirmedFloating := false
	for _, e := range floatingEvents {
		if !e.IsConfirmedThisWeek() {
			unconfirmedFloating = true
			break
		}
	}

	if unconfirmedFloating {
		sb.WriteString("<b>⚡️ Плавающие (выбери день):</b>\n")
		for _, e := range floatingEvents {
			if !e.IsConfirmedThisWeek() {
				days := e.GetFloatingDays()
				var dayNames []string
				for _, d := range days {
					dayNames = append(dayNames, domain.WeekdayNameShort(d))
				}
				sb.WriteString(fmt.Sprintf("  🔄 %s %s (%s)\n", e.TimeRange(), e.Title, strings.Join(dayNames, "/")))
			}
		}
		sb.WriteString("\n")
	}

	// Iterate through week starting from Monday
	daysOrder := []domain.Weekday{
		domain.WeekdayMonday, domain.WeekdayTuesday, domain.WeekdayWednesday,
		domain.WeekdayThursday, domain.WeekdayFriday, domain.WeekdaySaturday, domain.WeekdaySunday,
	}

	today := domain.Weekday(time.Now().Weekday())

	for _, day := range daysOrder {
		dayEvents := byDay[day]
		if len(dayEvents) == 0 {
			continue
		}

		// Day header
		todayMarker := ""
		if day == today {
			todayMarker = " ← сегодня"
		}
		sb.WriteString(fmt.Sprintf("<b>%s %s</b>%s\n", domain.WeekdayEmoji(day), domain.WeekdayName(day), todayMarker))

		for _, e := range dayEvents {
			timeStr := e.TimeRange()
			if timeStr == "" {
				timeStr = "—"
			}
			marks := ""
			if e.IsFloating {
				marks += " 🔄"
			}
			if e.IsShared {
				marks += " 👨‍👩‍👧‍👦"
			}
			if showIDs {
				sb.WriteString(fmt.Sprintf("  <code>#%d</code> %s %s%s\n", e.ID, timeStr, e.Title, marks))
			} else {
				sb.WriteString(fmt.Sprintf("  %s %s%s\n", timeStr, e.Title, marks))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatDaySchedule formats events for a single day
func (s *ScheduleService) FormatDaySchedule(events []*domain.WeeklyEvent, day domain.Weekday) string {
	if len(events) == 0 {
		return fmt.Sprintf("На %s событий нет", domain.WeekdayName(day))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>%s %s</b>\n\n", domain.WeekdayEmoji(day), domain.WeekdayName(day)))

	for _, e := range events {
		timeStr := e.TimeRange()
		if timeStr == "" {
			timeStr = "весь день"
		}
		sb.WriteString(fmt.Sprintf("🕐 %s — %s\n", timeStr, e.Title))
	}

	return sb.String()
}
