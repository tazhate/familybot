package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tazhate/familybot/config"
	"github.com/tazhate/familybot/internal/clients/debtmanager"
	"github.com/tazhate/familybot/internal/service"
	"github.com/tazhate/familybot/internal/storage"
)

type Bot struct {
	api              *tgbotapi.BotAPI
	cfg              *config.Config
	storage          *storage.Storage
	taskService      *service.TaskService
	reminderService  *service.ReminderService
	personService    *service.PersonService
	scheduleService  *service.ScheduleService
	autoService      *service.AutoService
	checklistService *service.ChecklistService
	calendarService  *service.CalendarService
	todoistService   *service.TodoistService
	debtClient       *debtmanager.Client
	server           *http.Server

	// Pending task text by chatID (for priority selection)
	pendingTasks   map[int64]string
	pendingTasksMu sync.RWMutex
}

func New(cfg *config.Config, storage *storage.Storage, taskSvc *service.TaskService, reminderSvc *service.ReminderService, personSvc *service.PersonService, scheduleSvc *service.ScheduleService, autoSvc *service.AutoService, checklistSvc *service.ChecklistService, calendarSvc *service.CalendarService, todoistSvc *service.TodoistService, debtClient *debtmanager.Client) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("create bot api: %w", err)
	}

	log.Printf("Authorized as @%s", api.Self.UserName)

	bot := &Bot{
		api:              api,
		cfg:              cfg,
		storage:          storage,
		taskService:      taskSvc,
		reminderService:  reminderSvc,
		personService:    personSvc,
		scheduleService:  scheduleSvc,
		autoService:      autoSvc,
		checklistService: checklistSvc,
		calendarService:  calendarSvc,
		todoistService:   todoistSvc,
		debtClient:       debtClient,
		pendingTasks:     make(map[int64]string),
	}

	// Set bot commands (menu button)
	bot.setCommands()

	return bot, nil
}

func (b *Bot) setCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "menu", Description: "📱 Главное меню"},
		{Command: "list", Description: "📋 Список задач"},
		{Command: "add", Description: "➕ Добавить задачу"},
		{Command: "today", Description: "📅 Задачи на сегодня"},
		{Command: "calendar", Description: "📆 Календарь"},
		{Command: "week", Description: "🗓 Недельное расписание"},
		{Command: "help", Description: "❓ Справка по командам"},
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)
	if _, err := b.api.Request(cfg); err != nil {
		log.Printf("Failed to set commands: %v", err)
	}
}

func (b *Bot) SetupWebhook() error {
	webhookURL := b.cfg.WebhookURL + "/bot"

	wh, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}

	_, err = b.api.Request(wh)
	if err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}

	info, err := b.api.GetWebhookInfo()
	if err != nil {
		return fmt.Errorf("get webhook info: %w", err)
	}

	if info.LastErrorDate != 0 {
		log.Printf("Webhook last error: %s", info.LastErrorMessage)
	}

	log.Printf("Webhook set to: %s", webhookURL)
	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	updates := b.api.ListenForWebhook("/bot")

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Setup REST API with Basic Auth
	b.SetupAPI()

	b.server = &http.Server{
		Addr:    ":" + b.cfg.ServerPort,
		Handler: nil, // use DefaultServeMux
	}

	go func() {
		log.Printf("Starting webhook server on :%s", b.cfg.ServerPort)
		if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case update := <-updates:
			go b.handleUpdate(update)
		}
	}
}

func (b *Bot) Stop(ctx context.Context) error {
	if b.server != nil {
		return b.server.Shutdown(ctx)
	}
	return nil
}

func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = persistentMenuKeyboard()
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) SendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboard
	_, err := b.api.Send(msg)
	return err
}

// SendMessageWithSnooze sends a reminder message with snooze buttons
func (b *Bot) SendMessageWithSnooze(chatID int64, text string, taskID int64) error {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", taskID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ +1 час", fmt.Sprintf("snooze:%d:1h", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("🌅 Завтра", fmt.Sprintf("snooze:%d:tomorrow", taskID)),
		),
	)
	return b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

// SetPendingTask stores task text for priority selection
func (b *Bot) SetPendingTask(chatID int64, text string) {
	b.pendingTasksMu.Lock()
	defer b.pendingTasksMu.Unlock()
	b.pendingTasks[chatID] = text
}

// GetPendingTask retrieves and removes pending task text
func (b *Bot) GetPendingTask(chatID int64) (string, bool) {
	b.pendingTasksMu.Lock()
	defer b.pendingTasksMu.Unlock()
	text, ok := b.pendingTasks[chatID]
	if ok {
		delete(b.pendingTasks, chatID)
	}
	return text, ok
}
