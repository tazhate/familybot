package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tazhate/familybot/internal/domain"
)

// Persistent reply keyboard (always visible at bottom)
func persistentMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📋 Задачи"),
			tgbotapi.NewKeyboardButton("📅 Сегодня"),
			tgbotapi.NewKeyboardButton("➕ Добавить"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🗓 Расписание"),
			tgbotapi.NewKeyboardButton("📆 Календарь"),
			tgbotapi.NewKeyboardButton("📱 Меню"),
		),
	)
}

// People keyboard
func peopleKeyboard(persons []*domain.Person) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Person buttons (show first 5)
	for i, p := range persons {
		if i >= 5 {
			break
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s %s", p.RoleEmoji(), p.Name),
				fmt.Sprintf("person:%d", p.ID),
			),
			tgbotapi.NewInlineKeyboardButtonData("🗑", fmt.Sprintf("del_person:%d", p.ID)),
		)
		rows = append(rows, row)
	}

	// Action row
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add_person"),
		tgbotapi.NewInlineKeyboardButtonData("🎂 ДР", "menu:birthdays"),
	))

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// Priority selection keyboard (text stored in bot.pendingTasks)
func priorityKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔴 Срочно", "setpri:urgent"),
			tgbotapi.NewInlineKeyboardButtonData("🟡 На неделе", "setpri:week"),
			tgbotapi.NewInlineKeyboardButtonData("🟢 Когда-нибудь", "setpri:someday"),
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
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👨‍👩‍👧 Сделать общей", fmt.Sprintf("share:%d", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
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
			tgbotapi.NewInlineKeyboardButtonData("🗓 Расписание", "menu:week"),
			tgbotapi.NewInlineKeyboardButtonData("🎂 ДР", "menu:birthdays"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Люди", "menu:people"),
			tgbotapi.NewInlineKeyboardButtonData("🚗 Машины", "menu:autos"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔔 Напоминания", "menu:reminders"),
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add"),
		),
	)
}

// Week schedule keyboard
func weekScheduleKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add_weekly"),
			tgbotapi.NewInlineKeyboardButtonData("🔄 Плавающие", "menu:floating"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
			tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu:main"),
		),
	)
}

// Floating event keyboard - for selecting day
func floatingEventKeyboard(event *domain.WeeklyEvent) tgbotapi.InlineKeyboardMarkup {
	days := event.GetFloatingDays()
	var buttons []tgbotapi.InlineKeyboardButton

	for _, d := range days {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
			domain.WeekdayNameShort(d),
			fmt.Sprintf("confirm_float:%d:%d", event.ID, d),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		buttons,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
		),
	)
}

// Floating list keyboard
func floatingListKeyboard(events []*domain.WeeklyEvent) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Show each event with day selection buttons
	for _, e := range events {
		if !e.IsConfirmedThisWeek() {
			days := e.GetFloatingDays()
			var dayButtons []tgbotapi.InlineKeyboardButton

			// Event title button
			titleBtn := tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🔄 %s:", truncate(e.Title, 15)),
				fmt.Sprintf("floating:%d", e.ID),
			)
			dayButtons = append(dayButtons, titleBtn)

			// Day buttons
			for _, d := range days {
				dayButtons = append(dayButtons, tgbotapi.NewInlineKeyboardButtonData(
					domain.WeekdayNameShort(d),
					fmt.Sprintf("confirm_float:%d:%d", e.ID, d),
				))
			}
			rows = append(rows, dayButtons)
		}
	}

	// Add navigation buttons
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add_floating"),
		tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
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
				fmt.Sprintf("⬜ %s", truncate(t.Title, 30)),
				fmt.Sprintf("done_today:%d", t.ID),
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

// Checklist keyboard - shows items as checkable buttons
func checklistKeyboard(c *domain.Checklist) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for i, item := range c.Items {
		status := "⬜"
		if item.Checked {
			status = "✅"
		}
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s %s", status, truncate(item.Text, 30)),
				fmt.Sprintf("cl_check:%d:%d", c.ID, i),
			),
		)
		rows = append(rows, row)
	}

	// Action row
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Сбросить", fmt.Sprintf("cl_reset:%d", c.ID)),
		tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("cl_del:%d", c.ID)),
	))

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Все чек-листы", "menu:checklists"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// Edit task keyboard
func editTaskKeyboard(taskID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔴 Срочно", fmt.Sprintf("pri:%d:urgent", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("🟡 Неделя", fmt.Sprintf("pri:%d:week", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("🟢 Потом", fmt.Sprintf("pri:%d:someday", taskID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Завтра", fmt.Sprintf("date:%d:tomorrow", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("📅 +Неделя", fmt.Sprintf("date:%d:week", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("📅 Убрать", fmt.Sprintf("date:%d:clear", taskID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", taskID)),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("del:%d", taskID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
		),
	)
}

// Checklists list keyboard
func checklistsListKeyboard(checklists []*domain.Checklist) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, c := range checklists {
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("📋 %s (%d/%d)", c.Title, c.CheckedCount(), len(c.Items)),
				fmt.Sprintf("cl_view:%d", c.ID),
			),
		)
		rows = append(rows, row)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Создать", "add_checklist"),
		tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu:main"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
