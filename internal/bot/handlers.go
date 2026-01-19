package bot

import (
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	// Проверяем доступ
	if !b.cfg.IsAllowedUser(userID) {
		b.SendMessage(chatID, "⛔ Доступ запрещён")
		return
	}

	// Проверяем/создаём пользователя
	user, err := b.storage.GetUserByTelegramID(userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Обработка команд
	if msg.IsCommand() {
		b.handleCommand(msg, user)
		return
	}

	// Простое добавление задачи текстом
	if user != nil {
		task, err := b.taskService.Create(user.ID, text, "")
		if err != nil {
			b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
			return
		}
		b.SendMessage(chatID, "✅ Задача добавлена: #"+itoa(task.ID))
	}
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

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

		// Обновляем сообщение
		if callback.Message != nil {
			tasks, _ := b.taskService.List(user.ID, false)
			text := b.taskService.FormatTaskList(tasks)
			edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, text)
			edit.ReplyMarkup = b.buildTaskListKeyboard(tasks)
			b.api.Send(edit)
		}

	case "priority":
		if len(parts) < 2 {
			return
		}
		// Ответ на выбор приоритета будет обрабатываться в состоянии сессии
		b.api.Request(tgbotapi.NewCallback(callback.ID, "Приоритет: "+parts[1]))

	default:
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
	}
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
