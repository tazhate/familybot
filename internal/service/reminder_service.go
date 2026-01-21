package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type ReminderService struct {
	storage  *storage.Storage
	timezone *time.Location
}

func NewReminderService(s *storage.Storage, tz *time.Location) *ReminderService {
	return &ReminderService{
		storage:  s,
		timezone: tz,
	}
}

func (s *ReminderService) Create(userID int64, title string, reminderType domain.ReminderType, params domain.ReminderParams) (*domain.Reminder, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("reminder title cannot be empty")
	}

	schedule, err := s.buildCronSchedule(reminderType, params)
	if err != nil {
		return nil, fmt.Errorf("build schedule: %w", err)
	}

	paramsJSON, _ := json.Marshal(params)

	nextRun, err := s.calculateNextRun(schedule)
	if err != nil {
		return nil, fmt.Errorf("calculate next run: %w", err)
	}

	reminder := &domain.Reminder{
		UserID:   userID,
		Title:    title,
		Type:     reminderType,
		Schedule: schedule,
		Params:   string(paramsJSON),
		IsActive: true,
		NextRun:  &nextRun,
	}

	if err := s.storage.CreateReminder(reminder); err != nil {
		return nil, fmt.Errorf("create reminder: %w", err)
	}

	return reminder, nil
}

func (s *ReminderService) buildCronSchedule(reminderType domain.ReminderType, params domain.ReminderParams) (string, error) {
	timeStr := params.Time
	if timeStr == "" {
		timeStr = "11:00"
	}
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid time format: %s", timeStr)
	}
	hour, minute := parts[0], parts[1]

	switch reminderType {
	case domain.ReminderDaily:
		// Каждый день в указанное время
		return fmt.Sprintf("%s %s * * *", minute, hour), nil

	case domain.ReminderWeekly:
		// Еженедельно в указанный день
		return fmt.Sprintf("%s %s * * %d", minute, hour, params.DayOfWeek), nil

	case domain.ReminderMonthly:
		// Ежемесячно в указанный день
		return fmt.Sprintf("%s %s %d * *", minute, hour, params.DayOfMonth), nil

	case domain.ReminderYearly:
		// Ежегодно
		return fmt.Sprintf("%s %s %d %d *", minute, hour, params.Day, params.Month), nil

	case domain.ReminderMonthWeek:
		// N-я неделя месяца, определённый день
		// Cron не поддерживает это напрямую, используем ближайшее приближение
		// Будем проверять программно при выполнении
		return fmt.Sprintf("%s %s * * %d", minute, hour, params.DayOfWeek), nil

	case domain.ReminderFloating:
		// Плавающие — без автоматического расписания
		return fmt.Sprintf("%s %s * * *", minute, hour), nil

	default:
		return "", fmt.Errorf("unknown reminder type: %s", reminderType)
	}
}

func (s *ReminderService) calculateNextRun(schedule string) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse schedule: %w", err)
	}

	now := time.Now().In(s.timezone)
	return sched.Next(now), nil
}

func (s *ReminderService) List(userID int64) ([]*domain.Reminder, error) {
	return s.storage.ListRemindersByUser(userID)
}

func (s *ReminderService) Get(reminderID int64) (*domain.Reminder, error) {
	return s.storage.GetReminder(reminderID)
}

func (s *ReminderService) GetDueReminders() ([]*domain.Reminder, error) {
	now := time.Now().In(s.timezone)
	return s.storage.ListDueReminders(now)
}

func (s *ReminderService) MarkSent(reminderID int64) error {
	reminder, err := s.storage.GetReminder(reminderID)
	if err != nil {
		return fmt.Errorf("get reminder: %w", err)
	}
	if reminder == nil {
		return fmt.Errorf("reminder not found")
	}

	nextRun, err := s.calculateNextRun(reminder.Schedule)
	if err != nil {
		return fmt.Errorf("calculate next run: %w", err)
	}

	now := time.Now().In(s.timezone)
	return s.storage.UpdateReminderNextRun(reminderID, now, nextRun)
}

func (s *ReminderService) Delete(reminderID int64, userID int64) error {
	reminder, err := s.storage.GetReminder(reminderID)
	if err != nil {
		return fmt.Errorf("get reminder: %w", err)
	}
	if reminder == nil {
		return fmt.Errorf("reminder not found")
	}

	if reminder.UserID != userID {
		return fmt.Errorf("access denied")
	}

	return s.storage.DeleteReminder(reminderID)
}

func (s *ReminderService) FormatReminderList(reminders []*domain.Reminder) string {
	if len(reminders) == 0 {
		return "Нет напоминаний"
	}

	var sb strings.Builder
	for _, r := range reminders {
		status := "🔔"
		if !r.IsActive {
			status = "🔕"
		}
		nextStr := "—"
		if r.NextRun != nil {
			nextStr = r.NextRun.In(s.timezone).Format("02.01.06 15:04")
		}
		sb.WriteString(fmt.Sprintf("%s #%d %s (след: %s)\n", status, r.ID, r.Title, nextStr))
	}
	return sb.String()
}
