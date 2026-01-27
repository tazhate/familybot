package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tazhate/familybot/config"
	"github.com/tazhate/familybot/internal/bot"
	"github.com/tazhate/familybot/internal/clients/caldav"
	"github.com/tazhate/familybot/internal/clients/debtmanager"
	"github.com/tazhate/familybot/internal/clients/todoist"
	"github.com/tazhate/familybot/internal/scheduler"
	"github.com/tazhate/familybot/internal/service"
	"github.com/tazhate/familybot/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Загрузка конфига
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализация storage
	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer store.Close()

	// Инициализация сервисов
	taskSvc := service.NewTaskService(store)
	reminderSvc := service.NewReminderService(store, cfg.Timezone)
	personSvc := service.NewPersonService(store)
	personSvc.SetReminderService(reminderSvc) // для автосоздания напоминаний о ДР
	scheduleSvc := service.NewScheduleService(store)
	autoSvc := service.NewAutoService(store)
	checklistSvc := service.NewChecklistService(store)

	// Инициализация клиента Debt Manager (опционально)
	var debtClient *debtmanager.Client
	if cfg.DebtManagerURL != "" && cfg.DebtManagerToken != "" {
		debtClient = debtmanager.NewClient(cfg.DebtManagerURL, cfg.DebtManagerToken)
		log.Printf("Debt Manager client configured: %s", cfg.DebtManagerURL)
	}

	// Инициализация CalDAV клиента и CalendarService (опционально)
	var calendarSvc *service.CalendarService
	if cfg.CalDAVUsername != "" && cfg.CalDAVPassword != "" {
		caldavClient := caldav.NewClient(cfg.CalDAVURL, cfg.CalDAVUsername, cfg.CalDAVPassword)
		if cfg.CalDAVCalendarID != "" {
			caldavClient.SetCalendarID(cfg.CalDAVCalendarID)
		}

		// Get owner user ID
		ownerUser, err := store.GetUserByTelegramID(cfg.OwnerTelegramID)
		if err == nil && ownerUser != nil {
			calendarSvc = service.NewCalendarService(store, caldavClient, ownerUser.ID, cfg.Timezone)
			if cfg.CalDAVCalendarID != "" {
				calendarSvc.SetCalendarPath(cfg.CalDAVCalendarID)
			}
			log.Printf("Apple Calendar (CalDAV) configured: %s", cfg.CalDAVURL)
		} else {
			log.Printf("Warning: CalDAV configured but owner user not found in DB")
		}
	}

	// Инициализация Todoist клиента и TodoistService (опционально)
	var todoistSvc *service.TodoistService
	if cfg.TodoistToken != "" {
		todoistClient := todoist.NewClient(cfg.TodoistToken)
		if cfg.TodoistProjectID != "" {
			todoistClient.SetProjectID(cfg.TodoistProjectID)
		}
		if cfg.TodoistSectionID != "" {
			todoistClient.SetSectionID(cfg.TodoistSectionID)
		}
		if cfg.TodoistPartnerSectionID != "" {
			todoistClient.SetPartnerSectionID(cfg.TodoistPartnerSectionID)
		}

		// Get owner user ID
		ownerUser, err := store.GetUserByTelegramID(cfg.OwnerTelegramID)
		if err == nil && ownerUser != nil {
			// Get partner user ID (if exists)
			var partnerUserID int64
			if cfg.PartnerTelegramID != 0 {
				partnerUser, err := store.GetUserByTelegramID(cfg.PartnerTelegramID)
				if err == nil && partnerUser != nil {
					partnerUserID = partnerUser.ID
				}
			}
			todoistSvc = service.NewTodoistService(store, todoistClient, ownerUser.ID, partnerUserID)
			log.Printf("Todoist configured (project: %s, owner section: %s, partner section: %s)", cfg.TodoistProjectID, cfg.TodoistSectionID, cfg.TodoistPartnerSectionID)
		} else {
			log.Printf("Warning: Todoist configured but owner user not found in DB")
		}
	}

	// Инициализация бота
	tgBot, err := bot.New(cfg, store, taskSvc, reminderSvc, personSvc, scheduleSvc, autoSvc, checklistSvc, calendarSvc, todoistSvc, debtClient)
	if err != nil {
		log.Fatalf("Failed to init bot: %v", err)
	}

	// Настройка webhook
	if err := tgBot.SetupWebhook(); err != nil {
		log.Fatalf("Failed to setup webhook: %v", err)
	}

	// Инициализация scheduler
	sched := scheduler.New(cfg, store, taskSvc, reminderSvc, personSvc, scheduleSvc, checklistSvc, calendarSvc, todoistSvc, debtClient)
	sched.SetSender(tgBot)

	// Контекст для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запуск scheduler в горутине
	go func() {
		if err := sched.Start(ctx); err != nil {
			log.Printf("Scheduler error: %v", err)
		}
	}()

	// Запуск бота в горутине
	go func() {
		if err := tgBot.Start(ctx); err != nil {
			log.Printf("Bot error: %v", err)
		}
	}()

	log.Println("FamilyBot started")

	// Ожидание сигнала завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	// Graceful shutdown
	cancel()
	sched.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := tgBot.Stop(shutdownCtx); err != nil {
		log.Printf("Error stopping bot: %v", err)
	}

	log.Println("FamilyBot stopped")
}
