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
	default:
		b.SendMessage(chatID, "Неизвестная команда. /help для списка команд")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	user, _ := b.storage.GetUserByTelegramID(userID)
	if user != nil {
		b.SendMessage(chatID, fmt.Sprintf("👋 С возвращением, %s!", user.Name))
		return
	}

	// Создаём пользователя
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

	b.SendMessage(chatID, fmt.Sprintf("👋 Привет, %s!\n\nЯ помогу управлять задачами и напоминаниями.\n\n/help — список команд", name))
}

func (b *Bot) cmdHelp(chatID int64) {
	text := `<b>Команды:</b>

<b>Задачи</b>
/add текст — добавить задачу
/list — список задач
/done ID — выполнить задачу
/today — задачи на сегодня

<b>Напоминания</b>
/reminders — список напоминаний

<b>Другое</b>
/help — эта справка

💡 Просто отправь текст — добавлю как задачу`

	b.SendMessage(chatID, text)
}

func (b *Bot) cmdAdd(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи текст задачи: /add Купить молоко")
		return
	}

	// Парсим приоритет из тегов
	priority := domain.PrioritySomeday
	if strings.Contains(args, "!срочно") || strings.Contains(args, "!urgent") {
		priority = domain.PriorityUrgent
		args = strings.ReplaceAll(args, "!срочно", "")
		args = strings.ReplaceAll(args, "!urgent", "")
	} else if strings.Contains(args, "!неделя") || strings.Contains(args, "!week") {
		priority = domain.PriorityWeek
		args = strings.ReplaceAll(args, "!неделя", "")
		args = strings.ReplaceAll(args, "!week", "")
	}

	task, err := b.taskService.Create(user.ID, strings.TrimSpace(args), priority)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Задача добавлена\n\n%s #%d %s", task.PriorityEmoji(), task.ID, task.Title)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", task.ID)),
		),
	)

	b.SendMessageWithKeyboard(chatID, text, keyboard)
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

	text := "<b>📋 Задачи:</b>\n\n" + b.taskService.FormatTaskList(tasks)

	if len(tasks) > 0 {
		keyboard := b.buildTaskListKeyboard(tasks)
		b.SendMessageWithKeyboard(chatID, text, *keyboard)
	} else {
		b.SendMessage(chatID, text)
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

	b.SendMessage(chatID, "✅ Задача #"+args+" выполнена!")
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

	text := "<b>📅 На сегодня:</b>\n\n" + b.taskService.FormatTaskList(tasks)
	b.SendMessage(chatID, text)
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

	text := "<b>🔔 Напоминания:</b>\n\n" + b.reminderService.FormatReminderList(reminders)
	b.SendMessage(chatID, text)
}

func (b *Bot) buildTaskListKeyboard(tasks []*domain.Task) *tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, t := range tasks {
		if t.IsDone() {
			continue
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ #%d %s", t.ID, truncate(t.Title, 20)),
				fmt.Sprintf("done:%d", t.ID),
			),
		)
		rows = append(rows, row)
		if len(rows) >= 5 {
			break
		}
	}

	if len(rows) == 0 {
		return nil
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &keyboard
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
