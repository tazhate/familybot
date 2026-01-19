package bot

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tazhate/familybot/internal/domain"
)

func (b *Bot) handleCommand(msg *tgbotapi.Message, user *domain.User) {
	chatID := msg.Chat.ID
	cmd := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())

	switch cmd {
	case "start":
		b.cmdStart(msg)
	case "help":
		b.cmdHelp(chatID)
	case "add":
		b.cmdAdd(chatID, user, args)
	case "list":
		b.cmdList(chatID, user)
	case "done":
		b.cmdDone(chatID, user, args)
	case "today":
		b.cmdToday(chatID, user)
	case "reminders":
		b.cmdReminders(chatID, user)
	case "menu":
		b.cmdMenu(chatID, user)
	default:
		b.SendMessage(chatID, "Неизвестная команда. /help для списка команд")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	user, _ := b.storage.GetUserByTelegramID(userID)
	if user != nil {
		text := fmt.Sprintf("👋 С возвращением, %s!", user.Name)
		kb := mainMenuKeyboard()
		b.SendMessageWithKeyboard(chatID, text, kb)
		return
	}

	name := msg.From.FirstName
	if msg.From.LastName != "" {
		name += " " + msg.From.LastName
	}

	role := domain.RoleOwner
	if userID == b.cfg.PartnerTelegramID {
		role = domain.RolePartner
	}

	newUser := &domain.User{
		TelegramID: userID,
		Name:       name,
		Role:       role,
	}

	if err := b.storage.CreateUser(newUser); err != nil {
		b.SendMessage(chatID, "❌ Ошибка регистрации: "+err.Error())
		return
	}

	text := fmt.Sprintf("👋 Привет, %s!\n\nЯ помогу управлять задачами и напоминаниями.", name)
	kb := mainMenuKeyboard()
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdHelp(chatID int64) {
	text := `<b>📚 Команды:</b>

<b>Задачи</b>
/add текст — добавить задачу
/list — список задач
/done ID — выполнить задачу
/today — задачи на сегодня

<b>Напоминания</b>
/reminders — список напоминаний

<b>Навигация</b>
/menu — главное меню
/help — эта справка

💡 <i>Просто отправь текст — добавлю как задачу</i>`

	kb := mainMenuKeyboard()
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdMenu(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks, _ := b.taskService.List(user.ID, false)
	urgentCount := 0
	for _, t := range tasks {
		if t.Priority == domain.PriorityUrgent {
			urgentCount++
		}
	}

	text := "<b>📱 Главное меню</b>\n\n"
	text += fmt.Sprintf("Активных задач: <b>%d</b>", len(tasks))
	if urgentCount > 0 {
		text += fmt.Sprintf(" (срочных: %d 🔴)", urgentCount)
	}

	kb := mainMenuKeyboard()
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdAdd(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Напиши текст задачи:")
		return
	}

	// Парсим приоритет из тегов
	priority := domain.Priority("")
	if strings.Contains(args, "!срочно") || strings.Contains(args, "!urgent") || strings.Contains(args, "!1") {
		priority = domain.PriorityUrgent
		args = strings.ReplaceAll(args, "!срочно", "")
		args = strings.ReplaceAll(args, "!urgent", "")
		args = strings.ReplaceAll(args, "!1", "")
	} else if strings.Contains(args, "!неделя") || strings.Contains(args, "!week") || strings.Contains(args, "!2") {
		priority = domain.PriorityWeek
		args = strings.ReplaceAll(args, "!неделя", "")
		args = strings.ReplaceAll(args, "!week", "")
		args = strings.ReplaceAll(args, "!2", "")
	} else if strings.Contains(args, "!потом") || strings.Contains(args, "!someday") || strings.Contains(args, "!3") {
		priority = domain.PrioritySomeday
		args = strings.ReplaceAll(args, "!потом", "")
		args = strings.ReplaceAll(args, "!someday", "")
		args = strings.ReplaceAll(args, "!3", "")
	}

	args = strings.TrimSpace(args)

	// Если приоритет не указан — показываем выбор
	if priority == "" {
		kb := priorityKeyboard(args)
		b.SendMessageWithKeyboard(chatID, "Выбери приоритет:\n\n<b>"+args+"</b>", kb)
		return
	}

	task, err := b.taskService.Create(user.ID, args, priority)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Задача добавлена\n\n%s <b>#%d</b> %s", task.PriorityEmoji(), task.ID, task.Title)
	kb := taskKeyboard(task.ID)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdList(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks, err := b.taskService.List(user.ID, false)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>📋 Задачи</b>\n\n"
	if len(tasks) == 0 {
		text += "Нет активных задач 🎉\n\nНажми ➕ чтобы добавить"
	} else {
		text += b.taskService.FormatTaskList(tasks)
	}

	kb := taskListKeyboard(tasks, 0)
	if kb != nil {
		b.SendMessageWithKeyboard(chatID, text, *kb)
	} else {
		// Empty state keyboard
		emptyKb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➕ Добавить задачу", "add"),
			),
		)
		b.SendMessageWithKeyboard(chatID, text, emptyKb)
	}
}

func (b *Bot) cmdDone(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID задачи: /done 1")
		return
	}

	taskID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	if err := b.taskService.MarkDone(taskID, user.ID); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "✅ Задача <b>#" + args + "</b> выполнена!"
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdToday(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks, err := b.taskService.ListForToday(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>📅 На сегодня</b>\n\n"
	if len(tasks) == 0 {
		text += "На сегодня задач нет! 🎉"
	} else {
		text += b.taskService.FormatTaskList(tasks)
	}

	kb := todayKeyboard(tasks)
	if kb != nil {
		b.SendMessageWithKeyboard(chatID, text, *kb)
	} else {
		b.SendMessage(chatID, text)
	}
}

func (b *Bot) cmdReminders(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	reminders, err := b.reminderService.List(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>🔔 Напоминания</b>\n\n" + b.reminderService.FormatReminderList(reminders)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К задачам", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
