package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tazhate/familybot/internal/domain"
)

// Priority selection keyboard
func priorityKeyboard(taskTitle string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔴 Срочно", "setpri:urgent:"+taskTitle),
			tgbotapi.NewInlineKeyboardButtonData("🟡 На неделе", "setpri:week:"+taskTitle),
			tgbotapi.NewInlineKeyboardButtonData("🟢 Когда-нибудь", "setpri:someday:"+taskTitle),
		),
	)
}

// Task action keyboard (for single task)
func taskKeyboard(taskID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("del:%d", taskID)),
		),
	)
}

// Task list keyboard with pagination
func taskListKeyboard(tasks []*domain.Task, page int) *tgbotapi.InlineKeyboardMarkup {
	if len(tasks) == 0 {
		return nil
	}

	const perPage = 5
	start := page * perPage
	end := start + perPage
	if end > len(tasks) {
		end = len(tasks)
	}

	var rows [][]tgbotapi.InlineKeyboardButton

	// Task buttons
	for _, t := range tasks[start:end] {
		if t.IsDone() {
			continue
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ %s #%d", t.PriorityEmoji(), t.ID),
				fmt.Sprintf("done:%d", t.ID),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				truncate(t.Title, 25),
				fmt.Sprintf("view:%d", t.ID),
			),
		)
		rows = append(rows, row)
	}

	// Pagination
	var navRow []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("page:%d", page-1)))
	}
	totalPages := (len(tasks) + perPage - 1) / perPage
	if page < totalPages-1 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("page:%d", page+1)))
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	// Action row
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add"),
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "refresh:list"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &keyboard
}

// View task keyboard
func viewTaskKeyboard(task *domain.Task) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if !task.IsDone() {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", task.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("del:%d", task.ID)),
		))

		// Priority change
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔴", fmt.Sprintf("pri:%d:urgent", task.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🟡", fmt.Sprintf("pri:%d:week", task.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🟢", fmt.Sprintf("pri:%d:someday", task.ID)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Назад к списку", "back:list"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// Confirm delete keyboard
func confirmDeleteKeyboard(taskID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Да, удалить", fmt.Sprintf("confirm_del:%d", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("◀️ Отмена", "back:list"),
		),
	)
}

// Main menu keyboard
func mainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
			tgbotapi.NewInlineKeyboardButtonData("📅 Сегодня", "menu:today"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔔 Напоминания", "menu:reminders"),
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add"),
		),
	)
}

// Today keyboard
func todayKeyboard(tasks []*domain.Task) *tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, t := range tasks {
		if t.IsDone() {
			continue
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ %s", truncate(t.Title, 30)),
				fmt.Sprintf("done:%d", t.ID),
			),
		)
		rows = append(rows, row)
		if len(rows) >= 10 {
			break
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Все задачи", "menu:list"),
		tgbotapi.NewInlineKeyboardButtonData("🔄", "refresh:today"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &keyboard
}
