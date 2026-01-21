package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tazhate/familybot/config"
	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/service"
	"github.com/tazhate/familybot/internal/storage"
)

type MessageSender interface {
	SendMessage(chatID int64, text string) error
	SendMessageWithSnooze(chatID int64, text string, taskID int64) error
}

type Scheduler struct {
	cron            *cron.Cron
	cfg             *config.Config
	storage         *storage.Storage
	taskService     *service.TaskService
	reminderService *service.ReminderService
	personService   *service.PersonService
	scheduleService *service.ScheduleService
	sender          MessageSender
}

func New(cfg *config.Config, storage *storage.Storage, taskSvc *service.TaskService, reminderSvc *service.ReminderService, personSvc *service.PersonService, scheduleSvc *service.ScheduleService) *Scheduler {
	location := cfg.Timezone

	c := cron.New(cron.WithLocation(location))

	return &Scheduler{
		cron:            c,
		cfg:             cfg,
		storage:         storage,
		taskService:     taskSvc,
		reminderService: reminderSvc,
		personService:   personSvc,
		scheduleService: scheduleSvc,
	}
}

func (s *Scheduler) SetSender(sender MessageSender) {
	s.sender = sender
}

func (s *Scheduler) Start(ctx context.Context) error {
	// Утренний брифинг (парсим "09:00" -> "0 9 * * *")
	morningSpec := timeToCron(s.cfg.MorningTime)
	if _, err := s.cron.AddFunc(morningSpec, s.morningBriefing); err != nil {
		return fmt.Errorf("add morning briefing: %w", err)
	}

	// Вечерний чекин
	eveningSpec := timeToCron(s.cfg.EveningTime)
	if _, err := s.cron.AddFunc(eveningSpec, s.eveningCheckin); err != nil {
		return fmt.Errorf("add evening checkin: %w", err)
	}

	// Проверка напоминаний каждую минуту
	if _, err := s.cron.AddFunc("* * * * *", s.checkReminders); err != nil {
		return fmt.Errorf("add reminder check: %w", err)
	}

	// Проверка напоминаний о событиях каждую минуту
	if _, err := s.cron.AddFunc("* * * * *", s.checkEventReminders); err != nil {
		return fmt.Errorf("add event reminder check: %w", err)
	}

	// Пятничное напоминание о плавающих событиях (в 10:00 по пятницам)
	if _, err := s.cron.AddFunc("0 10 * * 5", s.fridayFloatingReminder); err != nil {
		return fmt.Errorf("add friday floating reminder: %w", err)
	}

	// Проверка urgent задач каждый час (повторные напоминания)
	if _, err := s.cron.AddFunc("0 * * * *", s.checkUrgentTasks); err != nil {
		return fmt.Errorf("add urgent task check: %w", err)
	}

	// Проверка повторяющихся задач по времени каждую минуту
	if _, err := s.cron.AddFunc("* * * * *", s.checkRepeatingTasks); err != nil {
		return fmt.Errorf("add repeating task check: %w", err)
	}

	// Проверка напоминаний по задачам каждую минуту
	if _, err := s.cron.AddFunc("* * * * *", s.checkTaskReminders); err != nil {
		return fmt.Errorf("add task reminder check: %w", err)
	}

	s.cron.Start()
	log.Printf("Scheduler started (TZ: %s, morning: %s, evening: %s)",
		s.cfg.Timezone, s.cfg.MorningTime, s.cfg.EveningTime)

	<-ctx.Done()
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) morningBriefing() {
	if s.sender == nil {
		return
	}

	s.sendBriefingTo(s.cfg.OwnerTelegramID)
	if s.cfg.PartnerTelegramID != 0 {
		s.sendBriefingTo(s.cfg.PartnerTelegramID)
	}
}

func (s *Scheduler) sendBriefingTo(telegramID int64) {
	user, err := s.storage.GetUserByTelegramID(telegramID)
	if err != nil || user == nil {
		return
	}

	tasks, err := s.taskService.ListForToday(user.ID)
	if err != nil {
		log.Printf("Error getting today tasks: %v", err)
		return
	}

	text := "☀️ <b>Доброе утро!</b>\n\n"

	// Проверяем дни рождения
	birthdayText := s.checkBirthdays(user.ID)
	if birthdayText != "" {
		text += birthdayText + "\n"
	}

	if len(tasks) == 0 {
		text += "На сегодня задач нет. Отличный день!"
	} else {
		text += fmt.Sprintf("<b>На сегодня %d задач:</b>\n\n", len(tasks))
		text += s.taskService.FormatTaskList(tasks)
	}

	if err := s.sender.SendMessage(telegramID, text); err != nil {
		log.Printf("Error sending morning briefing to %d: %v", telegramID, err)
	}
}

// checkBirthdays returns birthday notifications text
func (s *Scheduler) checkBirthdays(userID int64) string {
	if s.personService == nil {
		return ""
	}

	// Получаем дни рождения на ближайшие 7 дней
	persons, err := s.personService.ListUpcomingBirthdays(userID, 7)
	if err != nil {
		log.Printf("Error getting birthdays: %v", err)
		return ""
	}

	if len(persons) == 0 {
		return ""
	}

	var result strings.Builder
	result.WriteString("🎂 <b>Дни рождения:</b>\n")

	for _, p := range persons {
		days := p.DaysUntilBirthday()
		age := ""
		if p.Birthday.Year() > 1 {
			nextAge := p.Age()
			if days > 0 {
				nextAge++
			}
			age = fmt.Sprintf(" (%d лет)", nextAge)
		}

		switch days {
		case 0:
			result.WriteString(fmt.Sprintf("🎉 <b>СЕГОДНЯ</b> — %s%s!\n", p.Name, age))
		case 1:
			result.WriteString(fmt.Sprintf("⏰ Завтра — %s%s\n", p.Name, age))
		default:
			result.WriteString(fmt.Sprintf("📅 Через %d дн. — %s%s\n", days, p.Name, age))
		}
	}

	return result.String()
}

func (s *Scheduler) eveningCheckin() {
	if s.sender == nil {
		return
	}

	s.sendCheckinTo(s.cfg.OwnerTelegramID)
	if s.cfg.PartnerTelegramID != 0 {
		s.sendCheckinTo(s.cfg.PartnerTelegramID)
	}
}

func (s *Scheduler) sendCheckinTo(telegramID int64) {
	user, err := s.storage.GetUserByTelegramID(telegramID)
	if err != nil || user == nil {
		return
	}

	// Получаем все невыполненные задачи
	tasks, err := s.taskService.List(user.ID, false)
	if err != nil {
		log.Printf("Error getting tasks: %v", err)
		return
	}

	urgentCount := 0
	for _, t := range tasks {
		if t.Priority == "urgent" {
			urgentCount++
		}
	}

	text := "🌙 <b>Вечерний чекин</b>\n\n"
	if len(tasks) == 0 {
		text += "Все задачи выполнены! Отдыхай 🎉"
	} else {
		text += fmt.Sprintf("Осталось задач: %d", len(tasks))
		if urgentCount > 0 {
			text += fmt.Sprintf(" (срочных: %d 🔴)", urgentCount)
		}
		text += "\n\n/list — посмотреть список"
	}

	if err := s.sender.SendMessage(telegramID, text); err != nil {
		log.Printf("Error sending evening checkin to %d: %v", telegramID, err)
	}
}

func (s *Scheduler) checkReminders() {
	if s.sender == nil {
		return
	}

	reminders, err := s.reminderService.GetDueReminders()
	if err != nil {
		log.Printf("Error getting due reminders: %v", err)
		return
	}

	for _, r := range reminders {
		user, err := s.storage.GetUserByID(r.UserID)
		if err != nil || user == nil {
			continue
		}

		text := fmt.Sprintf("🔔 <b>Напоминание</b>\n\n%s", r.Title)
		if err := s.sender.SendMessage(user.TelegramID, text); err != nil {
			log.Printf("Error sending reminder %d to user %d: %v", r.ID, user.TelegramID, err)
			continue
		}

		// Обновляем время следующего запуска
		if err := s.reminderService.MarkSent(r.ID); err != nil {
			log.Printf("Error marking reminder %d as sent: %v", r.ID, err)
		}
	}
}

func (s *Scheduler) checkEventReminders() {
	if s.sender == nil {
		return
	}

	events, err := s.storage.ListEventsWithReminders()
	if err != nil {
		log.Printf("Error getting events with reminders: %v", err)
		return
	}

	currentTime := time.Now().In(s.cfg.Timezone)
	currentWeekday := int(currentTime.Weekday())
	currentTimeStr := currentTime.Format("15:04")

	for _, e := range events {
		// Determine the effective day for this event
		eventDay := int(e.DayOfWeek)

		// For floating events, use confirmed day
		if e.IsFloating {
			if !e.IsConfirmedThisWeek() || e.ConfirmedDay == nil {
				continue
			}
			eventDay = *e.ConfirmedDay
		}

		// Skip if not today
		if eventDay != currentWeekday {
			continue
		}

		// Calculate reminder time
		eventTime, err := parseTime(e.TimeStart)
		if err != nil {
			continue
		}

		reminderTime := eventTime.Add(-time.Duration(e.ReminderBefore) * time.Minute)
		reminderTimeStr := reminderTime.Format("15:04")

		// Check if it's time to send reminder (exact minute match)
		if currentTimeStr != reminderTimeStr {
			continue
		}

		// Get user
		user, err := s.storage.GetUserByID(e.UserID)
		if err != nil || user == nil {
			continue
		}

		// Format the reminder text naturally
		var text string
		switch {
		case e.ReminderBefore >= 60 && e.ReminderBefore%60 == 0:
			hours := e.ReminderBefore / 60
			if hours == 1 {
				text = fmt.Sprintf("⏰ <b>Через 1 час</b> — %s (%s)", e.Title, e.TimeStart)
			} else {
				text = fmt.Sprintf("⏰ <b>Через %d ч</b> — %s (%s)", hours, e.Title, e.TimeStart)
			}
		case e.ReminderBefore > 0:
			text = fmt.Sprintf("⏰ <b>Через %d мин</b> — %s (%s)", e.ReminderBefore, e.Title, e.TimeStart)
		default:
			text = fmt.Sprintf("⏰ <b>Сейчас</b> — %s", e.Title)
		}

		if err := s.sender.SendMessage(user.TelegramID, text); err != nil {
			log.Printf("Error sending event reminder for event %d to user %d: %v", e.ID, user.TelegramID, err)
		}
	}
}

// fridayFloatingReminder sends reminders about floating events on Fridays
func (s *Scheduler) fridayFloatingReminder() {
	if s.sender == nil || s.scheduleService == nil {
		return
	}

	s.sendFloatingReminderTo(s.cfg.OwnerTelegramID)
	if s.cfg.PartnerTelegramID != 0 {
		s.sendFloatingReminderTo(s.cfg.PartnerTelegramID)
	}
}

func (s *Scheduler) sendFloatingReminderTo(telegramID int64) {
	user, err := s.storage.GetUserByTelegramID(telegramID)
	if err != nil || user == nil {
		return
	}

	events, err := s.scheduleService.ListFloating(user.ID)
	if err != nil {
		log.Printf("Error getting floating events: %v", err)
		return
	}

	// Filter only unconfirmed events for this week
	var unconfirmed []*domain.WeeklyEvent
	for _, e := range events {
		if !e.IsConfirmedThisWeek() {
			unconfirmed = append(unconfirmed, e)
		}
	}

	if len(unconfirmed) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("🔄 <b>Плавающие события на выходные:</b>\n\n")

	for _, e := range unconfirmed {
		days := e.GetFloatingDays()
		var dayNames []string
		for _, d := range days {
			dayNames = append(dayNames, domain.WeekdayNameShort(d))
		}
		sb.WriteString(fmt.Sprintf("• <b>%s</b> (%s) — %s\n", e.Title, strings.Join(dayNames, "/"), e.TimeRange()))
	}

	sb.WriteString("\nВыбери день: /floating")

	if err := s.sender.SendMessage(telegramID, sb.String()); err != nil {
		log.Printf("Error sending floating reminder to %d: %v", telegramID, err)
	}
}

// checkRepeatingTasks sends reminders for repeating tasks at specified time
func (s *Scheduler) checkRepeatingTasks() {
	if s.sender == nil || s.storage == nil {
		return
	}

	currentTime := time.Now().In(s.cfg.Timezone)
	currentTimeStr := currentTime.Format("15:04")
	currentWeekday := currentTime.Weekday()

	// Get all repeating tasks with this time
	tasks, err := s.storage.ListRepeatingTasksByTime(currentTimeStr)
	if err != nil {
		log.Printf("Error getting repeating tasks: %v", err)
		return
	}

	for _, task := range tasks {
		// Check if task should run today based on repeat type
		switch task.RepeatType {
		case domain.RepeatWeekdays:
			if currentWeekday == time.Saturday || currentWeekday == time.Sunday {
				continue
			}
		case domain.RepeatWeekly:
			// For weekly, only run on the same day as the due date
			if task.DueDate != nil && task.DueDate.Weekday() != currentWeekday {
				continue
			}
		case domain.RepeatMonthly:
			// For monthly, only run on the same day of month
			if task.DueDate != nil && task.DueDate.Day() != currentTime.Day() {
				continue
			}
		}

		// Get the user
		user, err := s.storage.GetUserByID(task.UserID)
		if err != nil || user == nil {
			continue
		}

		// Send reminder with snooze buttons
		text := fmt.Sprintf("🔁 <b>%s</b>\n\n%s #%d %s",
			currentTimeStr, task.PriorityEmoji(), task.ID, task.Title)

		if err := s.sender.SendMessageWithSnooze(user.TelegramID, text, task.ID); err != nil {
			log.Printf("Error sending repeating task reminder for task %d: %v", task.ID, err)
		}
	}
}

// checkUrgentTasks sends repeated reminders for urgent unfinished tasks
func (s *Scheduler) checkUrgentTasks() {
	if s.sender == nil || s.taskService == nil {
		return
	}

	tasks, err := s.taskService.ListUrgentForReminder()
	if err != nil {
		log.Printf("Error getting urgent tasks for reminder: %v", err)
		return
	}

	for _, task := range tasks {
		// Get the user to send reminder to
		var telegramID int64

		// First try assigned user
		if task.AssignedTo != nil {
			user, err := s.storage.GetUserByID(*task.AssignedTo)
			if err == nil && user != nil {
				telegramID = user.TelegramID
			}
		}

		// Fall back to creator
		if telegramID == 0 {
			user, err := s.storage.GetUserByID(task.UserID)
			if err == nil && user != nil {
				telegramID = user.TelegramID
			}
		}

		if telegramID == 0 {
			continue
		}

		// Format reminder
		reminderNum := task.ReminderCount + 1
		text := fmt.Sprintf("🔴 <b>Напоминание #%d</b>\n\nЗадача ждёт:\n<b>#%d</b> %s",
			reminderNum, task.ID, task.Title)

		if err := s.sender.SendMessageWithSnooze(telegramID, text, task.ID); err != nil {
			log.Printf("Error sending urgent task reminder for task %d to %d: %v", task.ID, telegramID, err)
			continue
		}

		// Mark as reminded
		if err := s.taskService.MarkReminded(task.ID); err != nil {
			log.Printf("Error marking task %d as reminded: %v", task.ID, err)
		}
	}
}

// parseTime parses "HH:MM" string to time.Time (today)
func parseTime(timeStr string) (time.Time, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format")
	}

	hour := 0
	min := 0
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &min)

	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location()), nil
}

// timeToCron converts "HH:MM" to cron spec "MM HH * * *"
func timeToCron(timeStr string) string {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return "0 9 * * *" // default 9:00
	}
	// Remove leading zeros for cron compatibility
	hour := strings.TrimLeft(parts[0], "0")
	if hour == "" {
		hour = "0"
	}
	minute := strings.TrimLeft(parts[1], "0")
	if minute == "" {
		minute = "0"
	}
	return fmt.Sprintf("%s %s * * *", minute, hour)
}

// checkTaskReminders sends reminders for tasks based on due_date - remind_before
func (s *Scheduler) checkTaskReminders() {
	if s.sender == nil || s.storage == nil {
		return
	}

	reminders, tasks, err := s.storage.GetPendingTaskReminders()
	if err != nil {
		log.Printf("Error getting pending task reminders: %v", err)
		return
	}

	for i, r := range reminders {
		task := tasks[i]

		// Get the user to send reminder to
		var telegramID int64

		// First try assigned user
		if task.AssignedTo != nil {
			user, err := s.storage.GetUserByID(*task.AssignedTo)
			if err == nil && user != nil {
				telegramID = user.TelegramID
			}
		}

		// Fall back to creator
		if telegramID == 0 {
			user, err := s.storage.GetUserByID(task.UserID)
			if err == nil && user != nil {
				telegramID = user.TelegramID
			}
		}

		if telegramID == 0 {
			continue
		}

		// Format reminder text
		intervalLabel := domain.RemindBeforeLabel(r.RemindBefore)
		dueStr := ""
		if task.DueDate != nil {
			dueStr = task.DueDate.In(s.cfg.Timezone).Format("02.01 15:04")
		}

		text := fmt.Sprintf("⏰ <b>Напоминание %s</b>\n\n%s <b>#%d</b> %s\n\n📅 Дедлайн: %s",
			intervalLabel, task.PriorityEmoji(), task.ID, task.Title, dueStr)

		if err := s.sender.SendMessageWithSnooze(telegramID, text, task.ID); err != nil {
			log.Printf("Error sending task reminder %d for task %d: %v", r.ID, task.ID, err)
			continue
		}

		// Mark as sent
		if err := s.storage.MarkTaskReminderSent(r.ID); err != nil {
			log.Printf("Error marking task reminder %d as sent: %v", r.ID, err)
		}
	}
}

