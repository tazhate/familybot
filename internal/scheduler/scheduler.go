package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tazhate/familybot/config"
	"github.com/tazhate/familybot/internal/clients/debtmanager"
	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/service"
	"github.com/tazhate/familybot/internal/storage"
)

type MessageSender interface {
	SendMessage(chatID int64, text string) error
	SendMessageWithSnooze(chatID int64, text string, taskID int64) error
}

type Scheduler struct {
	cron             *cron.Cron
	cfg              *config.Config
	storage          *storage.Storage
	taskService      *service.TaskService
	reminderService  *service.ReminderService
	personService    *service.PersonService
	scheduleService  *service.ScheduleService
	checklistService *service.ChecklistService
	calendarService  *service.CalendarService
	todoistService   *service.TodoistService
	debtClient       *debtmanager.Client
	sender           MessageSender
}

func New(cfg *config.Config, storage *storage.Storage, taskSvc *service.TaskService, reminderSvc *service.ReminderService, personSvc *service.PersonService, scheduleSvc *service.ScheduleService, checklistSvc *service.ChecklistService, calendarSvc *service.CalendarService, todoistSvc *service.TodoistService, debtClient *debtmanager.Client) *Scheduler {
	location := cfg.Timezone

	c := cron.New(cron.WithLocation(location))

	return &Scheduler{
		cron:             c,
		cfg:              cfg,
		storage:          storage,
		taskService:      taskSvc,
		reminderService:  reminderSvc,
		personService:    personSvc,
		scheduleService:  scheduleSvc,
		checklistService: checklistSvc,
		calendarService:  calendarSvc,
		todoistService:   todoistSvc,
		debtClient:       debtClient,
	}
}

func (s *Scheduler) SetSender(sender MessageSender) {
	s.sender = sender
}

func (s *Scheduler) Start(ctx context.Context) error {
	// Создание задач из отслеживаемых событий расписания (за 5 мин до брифинга)
	if _, err := s.cron.AddFunc("55 5 * * *", s.createTrackableEventTasks); err != nil {
		return fmt.Errorf("add trackable event tasks: %w", err)
	}

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

	// Apple Calendar: авто-синхронизация каждый час
	if s.calendarService != nil && s.calendarService.IsConfigured() {
		if _, err := s.cron.AddFunc("0 * * * *", s.syncAppleCalendar); err != nil {
			return fmt.Errorf("add apple calendar sync: %w", err)
		}
		// Проверка напоминаний о календарных событиях каждые 5 минут
		if _, err := s.cron.AddFunc("*/5 * * * *", s.checkCalendarEventReminders); err != nil {
			return fmt.Errorf("add calendar event reminders: %w", err)
		}
		log.Println("Apple Calendar sync enabled (hourly)")
	}

	// Todoist: авто-синхронизация каждый час
	if s.todoistService != nil && s.todoistService.IsConfigured() {
		if _, err := s.cron.AddFunc("30 * * * *", s.syncTodoist); err != nil {
			return fmt.Errorf("add todoist sync: %w", err)
		}
		log.Println("Todoist sync enabled (hourly)")
	}

	// Debt Manager: проверка платежей на завтра (вечером в 21:00)
	if s.debtClient != nil && s.debtClient.IsConfigured() {
		if _, err := s.cron.AddFunc("0 21 * * *", s.checkDebtPaymentsTomorrow); err != nil {
			return fmt.Errorf("add debt payments tomorrow check: %w", err)
		}
		// Debt Manager: проверка зарплаты и сводка платежей (утром в 10:00)
		if _, err := s.cron.AddFunc("0 10 * * *", s.checkPayday); err != nil {
			return fmt.Errorf("add payday check: %w", err)
		}
		log.Println("Debt Manager notifications enabled")
	}

	// Daily relationship quote at 12:00 (inspired by Imago therapy)
	if _, err := s.cron.AddFunc("0 12 * * *", s.sendDailyQuote); err != nil {
		return fmt.Errorf("add daily quote: %w", err)
	}
	log.Println("Daily relationship quotes enabled (12:00)")

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

	// Добавляем события календаря на сегодня
	if s.calendarService != nil {
		calendarEvents, err := s.calendarService.ListToday(user.ID)
		if err == nil && len(calendarEvents) > 0 {
			text += s.calendarService.FormatTodayBriefing(calendarEvents) + "\n"
		}
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

		// Append checklist if linked
		if e.ChecklistID != nil && s.checklistService != nil {
			checklist, err := s.checklistService.Get(*e.ChecklistID)
			if err == nil && checklist != nil {
				// Reset checklist before showing
				checklist.ResetChecks()
				text += "\n\n" + s.checklistService.FormatChecklist(checklist)
			}
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

// formatMoney formats a number with space as thousands separator (Russian style)
func formatMoney(amount float64) string {
	str := fmt.Sprintf("%.0f", amount)
	n := len(str)
	if n <= 3 {
		return str
	}
	var result strings.Builder
	remainder := n % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if n > remainder {
			result.WriteString(" ")
		}
	}
	for i := remainder; i < n; i += 3 {
		result.WriteString(str[i : i+3])
		if i+3 < n {
			result.WriteString(" ")
		}
	}
	return result.String()
}

// checkDebtPaymentsTomorrow sends notifications about debt payments due tomorrow
func (s *Scheduler) checkDebtPaymentsTomorrow() {
	if s.sender == nil || s.debtClient == nil || !s.debtClient.IsConfigured() {
		return
	}

	tomorrow := time.Now().In(s.cfg.Timezone).AddDate(0, 0, 1).Day()

	debts, err := s.debtClient.GetDebtsForDay(tomorrow)
	if err != nil {
		log.Printf("Error getting debts for tomorrow: %v", err)
		return
	}

	if len(debts) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("💳 <b>Платежи завтра:</b>\n\n")

	for _, d := range debts {
		emoji := debtmanager.CategoryEmoji(d.Category)
		sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n", emoji, d.Name))
		sb.WriteString(fmt.Sprintf("   %s ₽\n\n", formatMoney(d.MonthlyPayment)))
	}

	sb.WriteString("/debts — все долги")

	// Send only to owner (not partner)
	if err := s.sender.SendMessage(s.cfg.OwnerTelegramID, sb.String()); err != nil {
		log.Printf("Error sending debt payment reminder: %v", err)
	}
}

// checkPayday checks if today is a payday and sends summary of upcoming payments
func (s *Scheduler) checkPayday() {
	if s.sender == nil || s.debtClient == nil || !s.debtClient.IsConfigured() {
		return
	}

	today := time.Now().In(s.cfg.Timezone).Day()

	isPayday, incomes, err := s.debtClient.IsPayday(today)
	if err != nil {
		log.Printf("Error checking payday: %v", err)
		return
	}

	if !isPayday || len(incomes) == 0 {
		return
	}

	// Get all debts for payment summary
	debts, err := s.debtClient.GetDebts()
	if err != nil {
		log.Printf("Error getting debts for payday summary: %v", err)
		return
	}

	if len(debts) == 0 {
		return
	}

	var sb strings.Builder

	// Income info
	totalIncome := 0.0
	for _, inc := range incomes {
		totalIncome += inc.Amount
	}
	sb.WriteString(fmt.Sprintf("💰 <b>Зарплата!</b> %s ₽\n\n", formatMoney(totalIncome)))
	sb.WriteString("<b>Что платим в этом месяце:</b>\n\n")

	// List debts with payment days
	totalPayments := 0.0
	for _, d := range debts {
		if d.CurrentAmount <= 0 {
			continue
		}
		emoji := debtmanager.CategoryEmoji(d.Category)
		sb.WriteString(fmt.Sprintf("%s %s: %s ₽ (%d числа)\n", emoji, d.Name, formatMoney(d.MonthlyPayment), d.PaymentDay))
		totalPayments += d.MonthlyPayment
	}

	sb.WriteString(fmt.Sprintf("\n<b>Итого:</b> %s ₽\n", formatMoney(totalPayments)))
	remaining := totalIncome - totalPayments
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("💵 Остаётся: %s ₽\n", formatMoney(remaining)))
	} else if remaining < 0 {
		sb.WriteString(fmt.Sprintf("⚠️ Не хватает: %s ₽\n", formatMoney(-remaining)))
	}

	sb.WriteString("\n/debts — подробнее")

	// Send only to owner
	if err := s.sender.SendMessage(s.cfg.OwnerTelegramID, sb.String()); err != nil {
		log.Printf("Error sending payday summary: %v", err)
	}
}

// ============== Apple Calendar ==============

// syncAppleCalendar syncs events with Apple Calendar (runs hourly)
func (s *Scheduler) syncAppleCalendar() {
	if s.calendarService == nil || !s.calendarService.IsConfigured() {
		return
	}

	// Sync FROM Apple (calendar events)
	result, err := s.calendarService.SyncFromApple()
	if err != nil {
		log.Printf("Apple Calendar sync error: %v", err)
		return
	}

	if result.Added > 0 || result.Updated > 0 || result.Deleted > 0 {
		log.Printf("Apple Calendar sync from Apple: added=%d, updated=%d, deleted=%d",
			result.Added, result.Updated, result.Deleted)
	}

	// Sync TO Apple (weekly schedule events)
	if s.scheduleService != nil && s.storage != nil {
		user, _ := s.storage.GetUserByTelegramID(s.cfg.OwnerTelegramID)
		if user != nil {
			events, err := s.scheduleService.List(user.ID, true)
			if err == nil {
				synced := 0
				for _, e := range events {
					var floatingDays []int
					if e.IsFloating {
						for _, d := range e.GetFloatingDays() {
							floatingDays = append(floatingDays, int(d))
						}
					}
					if err := s.calendarService.SyncWeeklyEventToCalendar(e.ID, int(e.DayOfWeek), e.TimeStart, e.TimeEnd, e.Title, e.IsFloating, floatingDays); err == nil {
						synced++
					}
				}
				if synced > 0 {
					log.Printf("Apple Calendar sync to Apple: schedule=%d", synced)
				}
			}
		}
	}
}

// checkCalendarEventReminders sends reminders 30 minutes before calendar events
func (s *Scheduler) checkCalendarEventReminders() {
	if s.sender == nil || s.calendarService == nil {
		return
	}

	// Get events starting in the next 30 minutes
	events, err := s.calendarService.GetUpcomingForReminder(30)
	if err != nil {
		log.Printf("Error getting upcoming calendar events: %v", err)
		return
	}

	for _, e := range events {
		// Skip all-day events (no time reminder needed)
		if e.AllDay {
			continue
		}

		// Calculate minutes until event
		minutesUntil := int(time.Until(e.StartTime).Minutes())

		// Only send reminder once at ~30 minutes mark (between 28-32 minutes)
		if minutesUntil < 28 || minutesUntil > 32 {
			continue
		}

		// Get user for this event
		user, err := s.storage.GetUserByID(e.UserID)
		if err != nil || user == nil {
			continue
		}

		// Format time in configured timezone
		localTime := e.StartTime.In(s.cfg.Timezone).Format("15:04")

		// Format reminder text
		text := fmt.Sprintf("⏰ <b>Через 30 мин</b> — %s (%s)", e.Title, localTime)

		if e.Location != "" {
			text += fmt.Sprintf("\n📍 %s", e.Location)
		}

		// Send to owner
		if err := s.sender.SendMessage(user.TelegramID, text); err != nil {
			log.Printf("Error sending calendar reminder for event %d: %v", e.ID, err)
		}

		// Also send to partner if event is shared
		if e.IsShared && s.cfg.PartnerTelegramID != 0 && s.cfg.PartnerTelegramID != user.TelegramID {
			if err := s.sender.SendMessage(s.cfg.PartnerTelegramID, text); err != nil {
				log.Printf("Error sending calendar reminder to partner for event %d: %v", e.ID, err)
			}
		}
	}
}

// ============== Trackable Schedule Events ==============

// createTrackableEventTasks creates tasks from trackable schedule events for today
func (s *Scheduler) createTrackableEventTasks() {
	if s.scheduleService == nil || s.taskService == nil || s.storage == nil {
		return
	}

	s.createTrackableTasksForUser(s.cfg.OwnerTelegramID)
	if s.cfg.PartnerTelegramID != 0 {
		s.createTrackableTasksForUser(s.cfg.PartnerTelegramID)
	}
}

func (s *Scheduler) createTrackableTasksForUser(telegramID int64) {
	user, err := s.storage.GetUserByTelegramID(telegramID)
	if err != nil || user == nil {
		return
	}

	// Get trackable events for today
	today := domain.Weekday(time.Now().In(s.cfg.Timezone).Weekday())
	events, err := s.scheduleService.ListForDay(user.ID, today, false) // own events only
	if err != nil {
		log.Printf("Error getting schedule events for user %d: %v", user.ID, err)
		return
	}

	todayDate := time.Now().In(s.cfg.Timezone)
	todayStart := time.Date(todayDate.Year(), todayDate.Month(), todayDate.Day(), 0, 0, 0, 0, todayDate.Location())

	for _, e := range events {
		// Skip non-trackable events
		if !e.IsTrackable {
			continue
		}

		// Skip floating events not confirmed for today
		if e.IsFloating {
			if !e.IsConfirmedThisWeek() || e.ConfirmedDay == nil || domain.Weekday(*e.ConfirmedDay) != today {
				continue
			}
		}

		// Check if task already exists for this event today
		// We use a naming convention: task title starts with event title
		exists, err := s.storage.TaskExistsForEventToday(user.ID, e.Title, todayStart)
		if err != nil {
			log.Printf("Error checking task existence for event %d: %v", e.ID, err)
			continue
		}
		if exists {
			continue
		}

		// Create task from event
		dueDate := todayStart
		// If event has time, use that time for due date
		if e.TimeStart != "" {
			if t, err := parseTime(e.TimeStart); err == nil {
				dueDate = time.Date(todayDate.Year(), todayDate.Month(), todayDate.Day(), t.Hour(), t.Minute(), 0, 0, todayDate.Location())
			}
		}

		task := &domain.Task{
			UserID:   user.ID,
			Title:    e.Title,
			Priority: "week", // Default priority
			DueDate:  &dueDate,
		}

		if err := s.storage.CreateTask(task); err != nil {
			log.Printf("Error creating task from event %d: %v", e.ID, err)
			continue
		}

		log.Printf("Created task #%d from trackable event #%d: %s", task.ID, e.ID, task.Title)
	}
}

// ============== Todoist ==============

// syncTodoist syncs tasks with Todoist (runs hourly)
func (s *Scheduler) syncTodoist() {
	if s.todoistService == nil || !s.todoistService.IsConfigured() {
		return
	}

	result, err := s.todoistService.Sync()
	if err != nil {
		log.Printf("Todoist sync error: %v", err)
		return
	}

	if result.FromTodoist.Added > 0 || result.FromTodoist.Updated > 0 || result.FromTodoist.Deleted > 0 ||
		result.ToTodoist.Added > 0 || result.ToTodoist.Updated > 0 {
		log.Printf("Todoist sync: from_todoist(+%d, ~%d, -%d), to_todoist(+%d, ~%d)",
			result.FromTodoist.Added, result.FromTodoist.Updated, result.FromTodoist.Deleted,
			result.ToTodoist.Added, result.ToTodoist.Updated)
	}

	if len(result.Errors) > 0 {
		log.Printf("Todoist sync errors: %d", len(result.Errors))
	}
}

// ============== Daily Relationship Quotes ==============

// sendDailyQuote sends a daily relationship quote inspired by Imago therapy
func (s *Scheduler) sendDailyQuote() {
	if s.sender == nil {
		return
	}

	quote := domain.GetDailyQuote()

	var message string
	if quote.Author != "" {
		message = fmt.Sprintf("💕 <b>Цитата дня о любви</b>\n\n<i>\"%s\"</i>\n\n— %s", quote.Text, quote.Author)
	} else {
		message = fmt.Sprintf("💕 <b>Цитата дня о любви</b>\n\n<i>\"%s\"</i>", quote.Text)
	}

	// Send to group chat if configured, otherwise send to individuals
	if s.cfg.GroupChatID != 0 {
		if err := s.sender.SendMessage(s.cfg.GroupChatID, message); err != nil {
			log.Printf("Error sending daily quote to group chat: %v", err)
		}
		log.Printf("Daily quote sent to group: %s", quote.Text[:50])
	} else {
		// Fallback to individual messages
		if err := s.sender.SendMessage(s.cfg.OwnerTelegramID, message); err != nil {
			log.Printf("Error sending daily quote to owner: %v", err)
		}

		if s.cfg.PartnerTelegramID != 0 {
			if err := s.sender.SendMessage(s.cfg.PartnerTelegramID, message); err != nil {
				log.Printf("Error sending daily quote to partner: %v", err)
			}
		}
		log.Printf("Daily quote sent to individuals: %s", quote.Text[:50])
	}
}

