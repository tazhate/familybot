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

	// Инициализация бота
	tgBot, err := bot.New(cfg, store, taskSvc, reminderSvc, personSvc)
	if err != nil {
		log.Fatalf("Failed to init bot: %v", err)
	}

	// Настройка webhook
	if err := tgBot.SetupWebhook(); err != nil {
		log.Fatalf("Failed to setup webhook: %v", err)
	}

	// Инициализация scheduler
	sched := scheduler.New(cfg, store, taskSvc, reminderSvc)
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
