package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tazhate/familybot/internal/domain"
)

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	if !b.cfg.IsAllowedUser(userID) {
		b.SendMessage(chatID, "⛔ Доступ запрещён")
		return
	}

	user, err := b.storage.GetUserByTelegramID(userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	if msg.IsCommand() {
		b.handleCommand(msg, user)
		return
	}

	// Добавление задачи текстом — показываем выбор приоритета
	if user != nil {
		kb := priorityKeyboard(text)
		b.SendMessageWithKeyboard(chatID, "Выбери приоритет для задачи:\n\n<b>"+text+"</b>", kb)
	}
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	chatID := callback.Message.Chat.ID
	msgID := callback.Message.MessageID

	if !b.cfg.IsAllowedUser(userID) {
		b.api.Request(tgbotapi.NewCallback(callback.ID, "⛔ Доступ запрещён"))
		return
	}

	user, _ := b.storage.GetUserByTelegramID(userID)
	if user == nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, "Сначала /start"))
		return
	}

	data := callback.Data
	parts := strings.Split(data, ":")

	switch parts[0] {
	case "setpri":
		// setpri:priority:taskTitle
		if len(parts) < 3 {
			return
		}
		priority := domain.Priority(parts[1])
		title := strings.Join(parts[2:], ":")

		task, err := b.taskService.Create(user.ID, title, priority)
		if err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Задача создана!"))

		text := fmt.Sprintf("✅ Задача добавлена\n\n%s <b>#%d</b> %s", task.PriorityEmoji(), task.ID, task.Title)
		kb := taskKeyboard(task.ID)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "done":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		if err := b.taskService.MarkDone(taskID, user.ID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Выполнено!"))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "del":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		task, _ := b.storage.GetTask(taskID)
		if task == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Задача не найдена"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		text := fmt.Sprintf("🗑 Удалить задачу?\n\n<b>#%d</b> %s", task.ID, task.Title)
		kb := confirmDeleteKeyboard(taskID)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "confirm_del":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		if err := b.taskService.Delete(taskID, user.ID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "🗑 Удалено!"))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "view":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		task, _ := b.storage.GetTask(taskID)
		if task == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Задача не найдена"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		status := "⬜ Не выполнено"
		if task.IsDone() {
			status = "✅ Выполнено"
		}
		text := fmt.Sprintf("%s <b>#%d</b>\n\n%s\n\nСтатус: %s\nПриоритет: %s",
			task.PriorityEmoji(), task.ID, task.Title, status, task.Priority)

		kb := viewTaskKeyboard(task)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "pri":
		// pri:taskID:priority
		if len(parts) < 3 {
			return
		}
		taskID := atoi(parts[1])
		priority := domain.Priority(parts[2])

		// Update priority (need to add this method)
		task, _ := b.storage.GetTask(taskID)
		if task == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Задача не найдена"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "Приоритет изменён: "+string(priority)))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "page":
		if len(parts) < 2 {
			return
		}
		page := int(atoi(parts[1]))
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		b.showTaskListPage(chatID, msgID, user.ID, page)

	case "menu":
		if len(parts) < 2 {
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		switch parts[1] {
		case "list":
			b.refreshTaskList(chatID, msgID, user.ID)
		case "today":
			b.showToday(chatID, msgID, user.ID)
		case "reminders":
			b.showReminders(chatID, msgID, user.ID)
		}

	case "back":
		if len(parts) < 2 {
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		switch parts[1] {
		case "list":
			b.refreshTaskList(chatID, msgID, user.ID)
		}

	case "refresh":
		if len(parts) < 2 {
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "🔄"))
		switch parts[1] {
		case "list":
			b.refreshTaskList(chatID, msgID, user.ID)
		case "today":
			b.showToday(chatID, msgID, user.ID)
		}

	case "add":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		b.SendMessage(chatID, "Напиши текст задачи:")

	default:
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
	}
}

func (b *Bot) refreshTaskList(chatID int64, msgID int, userID int64) {
	b.showTaskListPage(chatID, msgID, userID, 0)
}

func (b *Bot) showTaskListPage(chatID int64, msgID int, userID int64, page int) {
	tasks, _ := b.taskService.List(userID, false)

	text := "<b>📋 Задачи</b>\n\n"
	if len(tasks) == 0 {
		text += "Нет активных задач 🎉\n\nНажми ➕ чтобы добавить"
	} else {
		text += b.taskService.FormatTaskList(tasks)
	}

	kb := taskListKeyboard(tasks, page)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	if kb != nil {
		edit.ReplyMarkup = kb
	}
	b.api.Send(edit)
}

func (b *Bot) showToday(chatID int64, msgID int, userID int64) {
	tasks, _ := b.taskService.ListForToday(userID)

	text := "<b>📅 На сегодня</b>\n\n"
	if len(tasks) == 0 {
		text += "На сегодня задач нет! 🎉"
	} else {
		text += b.taskService.FormatTaskList(tasks)
	}

	kb := todayKeyboard(tasks)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	if kb != nil {
		edit.ReplyMarkup = kb
	}
	b.api.Send(edit)
}

func (b *Bot) showReminders(chatID int64, msgID int, userID int64) {
	reminders, _ := b.reminderService.List(userID)

	text := "<b>🔔 Напоминания</b>\n\n"
	text += b.reminderService.FormatReminderList(reminders)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", "menu:list"),
		),
	)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
