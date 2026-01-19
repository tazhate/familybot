package scheduler

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/tazhate/familybot/config"
	"github.com/tazhate/familybot/internal/service"
	"github.com/tazhate/familybot/internal/storage"
)

type MessageSender interface {
	SendMessage(chatID int64, text string) error
}

type Scheduler struct {
	cron            *cron.Cron
	cfg             *config.Config
	storage         *storage.Storage
	taskService     *service.TaskService
	reminderService *service.ReminderService
	sender          MessageSender
}

func New(cfg *config.Config, storage *storage.Storage, taskSvc *service.TaskService, reminderSvc *service.ReminderService) *Scheduler {
	location := cfg.Timezone

	c := cron.New(cron.WithLocation(location))

	return &Scheduler{
		cron:            c,
		cfg:             cfg,
		storage:         storage,
		taskService:     taskSvc,
		reminderService: reminderSvc,
	}
}

func (s *Scheduler) SetSender(sender MessageSender) {
	s.sender = sender
}

func (s *Scheduler) Start(ctx context.Context) error {
	// Утренний брифинг
	morningSpec := fmt.Sprintf("0 %s * * *", s.cfg.MorningTime)
	if _, err := s.cron.AddFunc(morningSpec, s.morningBriefing); err != nil {
		return fmt.Errorf("add morning briefing: %w", err)
	}

	// Вечерний чекин
	eveningSpec := fmt.Sprintf("0 %s * * *", s.cfg.EveningTime)
	if _, err := s.cron.AddFunc(eveningSpec, s.eveningCheckin); err != nil {
		return fmt.Errorf("add evening checkin: %w", err)
	}

	// Проверка напоминаний каждую минуту
	if _, err := s.cron.AddFunc("* * * * *", s.checkReminders); err != nil {
		return fmt.Errorf("add reminder check: %w", err)
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

