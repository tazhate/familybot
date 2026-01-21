package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

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

	// Авто-регистрация если пользователь в allowed list но не зарегистрирован
	if user == nil {
		user = b.autoRegisterUser(msg.From)
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

// autoRegisterUser auto-registers an allowed user
func (b *Bot) autoRegisterUser(from *tgbotapi.User) *domain.User {
	name := from.FirstName
	if from.LastName != "" {
		name += " " + from.LastName
	}

	role := domain.RoleOwner
	if from.ID == b.cfg.PartnerTelegramID {
		role = domain.RolePartner
	}

	newUser := &domain.User{
		TelegramID: from.ID,
		Name:       name,
		Role:       role,
	}

	if err := b.storage.CreateUser(newUser); err != nil {
		log.Printf("Error auto-registering user: %v", err)
		return nil
	}

	log.Printf("Auto-registered user: %s (ID: %d)", name, from.ID)
	return newUser
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
		user = b.autoRegisterUser(callback.From)
		if user == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Ошибка регистрации"))
			return
		}
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

		// Парсим @упоминания
		cleanText, mentions := b.taskService.ParseMentions(title)
		var personID *int64
		for _, mention := range mentions {
			person, _ := b.personService.GetByName(user.ID, mention)
			if person != nil {
				personID = &person.ID
				break
			}
		}

		// Парсим дату из текста
		cleanText, dueDate := b.taskService.ParseDate(cleanText)

		task, err := b.taskService.CreateFull(user.ID, chatID, cleanText, priority, personID, dueDate)
		if err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Задача создана!"))

		text := fmt.Sprintf("✅ Задача добавлена\n\n%s <b>#%d</b> %s", task.PriorityEmoji(), task.ID, task.Title)
		if task.DueDate != nil {
			text += fmt.Sprintf("\n📅 %s", task.DueDate.Format("02.01.2006"))
		}
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
		if err := b.taskService.MarkDone(taskID, user.ID, chatID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Выполнено!"))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "done_today":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		if err := b.taskService.MarkDone(taskID, user.ID, chatID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Выполнено!"))
		b.showToday(chatID, msgID, user.ID)

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
		if err := b.taskService.Delete(taskID, user.ID, chatID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "🗑 Удалено!"))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "share":
		if len(parts) < 2 {
			return
		}
		taskID := atoi(parts[1])
		if err := b.taskService.SetShared(taskID, user.ID, chatID, true); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "👨‍👩‍👧 Задача стала общей!"))

		task, _ := b.storage.GetTask(taskID)
		if task != nil {
			text := fmt.Sprintf("👨‍👩‍👧 <b>Задача стала общей</b>\n\n%s <b>#%d</b> %s", task.PriorityEmoji(), task.ID, task.Title)
			kb := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✅ Выполнено", fmt.Sprintf("done:%d", taskID)),
					tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("👨‍👩‍👧 Все общие", "menu:shared"),
				),
			)
			edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
			edit.ParseMode = "HTML"
			edit.ReplyMarkup = &kb
			b.api.Send(edit)
		}

	case "snooze":
		// snooze:taskID:duration (1h or tomorrow)
		if len(parts) < 3 {
			return
		}
		taskID := atoi(parts[1])
		durationStr := parts[2]

		var duration time.Duration
		var responseText string
		switch durationStr {
		case "1h":
			duration = time.Hour
			responseText = "⏰ Отложено на 1 час"
		case "tomorrow":
			// Calculate time until tomorrow 9:00
			now := time.Now()
			tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, now.Location())
			duration = time.Until(tomorrow)
			responseText = "🌅 Отложено до завтра"
		default:
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Неверное время"))
			return
		}

		if err := b.taskService.Snooze(taskID, user.ID, chatID, duration); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, responseText))

		// Update the message to show it's snoozed
		task, _ := b.storage.GetTask(taskID)
		if task != nil {
			text := fmt.Sprintf("%s %s\n\n%s <b>#%d</b> %s", responseText, "✓", task.PriorityEmoji(), task.ID, task.Title)
			edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
			edit.ParseMode = "HTML"
			b.api.Send(edit)
		}

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

		if err := b.taskService.UpdatePriority(taskID, user.ID, chatID, priority); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ Приоритет: "+string(priority)))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "date":
		// date:taskID:value (tomorrow, week, clear)
		if len(parts) < 3 {
			return
		}
		taskID := atoi(parts[1])
		value := parts[2]

		var dueDate *time.Time
		var responseText string
		now := time.Now()

		switch value {
		case "tomorrow":
			t := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			dueDate = &t
			responseText = "📅 Завтра"
		case "week":
			t := time.Date(now.Year(), now.Month(), now.Day()+7, 0, 0, 0, 0, now.Location())
			dueDate = &t
			responseText = "📅 Через неделю"
		case "clear":
			dueDate = nil
			responseText = "📅 Дата убрана"
		default:
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Неизвестная дата"))
			return
		}

		if err := b.taskService.UpdateDueDate(taskID, user.ID, chatID, dueDate); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, responseText))
		b.refreshTaskList(chatID, msgID, user.ID)

	case "page":
		if len(parts) < 2 {
			return
		}
		page := int(atoi(parts[1]))
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		b.showTaskListPage(chatID, msgID, page, user.ID)

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
		case "people":
			b.showPeople(chatID, msgID, user.ID)
		case "birthdays":
			b.showBirthdays(chatID, msgID, user.ID)
		case "week":
			b.showWeekSchedule(chatID, msgID, user.ID)
		case "main":
			b.showMainMenu(chatID, msgID)
		case "floating":
			b.showFloating(chatID, msgID, user.ID)
		case "shared":
			b.showShared(chatID, msgID, user.ID)
		case "autos":
			b.showAutos(chatID, msgID, user.ID)
		case "checklists":
			b.showChecklists(chatID, msgID, user.ID)
		case "history":
			b.showHistory(chatID, msgID, user.ID)
		case "stats":
			b.showStats(chatID, msgID, user.ID)
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
		case "week":
			b.showWeekSchedule(chatID, msgID, user.ID)
		}

	case "add":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		b.SendMessage(chatID, "Напиши текст задачи:")

	case "add_weekly":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		text := `<b>Добавить регулярное событие:</b>

/addweekly День Время Название

<b>Примеры:</b>
/addweekly Пн 17:30 Федя спорт
/addweekly Ср 16:00-20:00 Тим плавание
/addweekly Сб 10:00 Шахматы

<b>Дни:</b> Пн, Вт, Ср, Чт, Пт, Сб, Вс`
		b.SendMessage(chatID, text)

	case "add_floating":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		text := `<b>Добавить плавающее событие:</b>

/addfloating Дни Время Название

<b>Примеры:</b>
/addfloating Сб,Вс 10:00 Лука
/addfloating Пт,Сб 19:00 Кино`
		b.SendMessage(chatID, text)

	case "confirm_float":
		// confirm_float:eventID:dayOfWeek
		if len(parts) < 3 {
			return
		}
		eventID := atoi(parts[1])
		dayOfWeek := domain.Weekday(atoi(parts[2]))

		if err := b.scheduleService.ConfirmFloatingDay(eventID, user.ID, dayOfWeek); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ "+domain.WeekdayName(dayOfWeek)))
		b.showWeekSchedule(chatID, msgID, user.ID)

	case "floating":
		// floating:eventID - show single floating event
		if len(parts) < 2 {
			return
		}
		eventID := atoi(parts[1])
		event, _ := b.scheduleService.Get(eventID)
		if event == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Не найдено"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		days := event.GetFloatingDays()
		var dayNames []string
		for _, d := range days {
			dayNames = append(dayNames, domain.WeekdayNameShort(d))
		}

		status := "❓ не выбран на эту неделю"
		if event.IsConfirmedThisWeek() && event.ConfirmedDay != nil {
			status = "✅ выбран " + domain.WeekdayName(domain.Weekday(*event.ConfirmedDay))
		}

		text := fmt.Sprintf("🔄 <b>%s</b>\n\nВремя: %s\nДни: %s\nСтатус: %s\n\n<b>Выбери день:</b>",
			event.Title, event.TimeRange(), strings.Join(dayNames, ", "), status)

		kb := floatingEventKeyboard(event)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "add_auto":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		text := `<b>Добавить машину:</b>

/addauto Название [Год]

<b>Примеры:</b>
/addauto Kia Rio 2020
/addauto Camry`
		b.SendMessage(chatID, text)

	case "add_person":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		text := `<b>Добавить человека:</b>

/addperson Имя роль ДД.ММ.ГГГГ

<b>Примеры:</b>
/addperson Тим ребёнок 12.06.2017
/addperson Ира семья 17.12
/addperson Федя контакт`
		b.SendMessage(chatID, text)

	case "del_person":
		if len(parts) < 2 {
			return
		}
		personID := atoi(parts[1])
		person, _ := b.personService.Get(personID)
		if person == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Не найден"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		text := fmt.Sprintf("🗑 Удалить <b>%s</b>?", person.Name)
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Да, удалить", fmt.Sprintf("confirm_del_person:%d", personID)),
				tgbotapi.NewInlineKeyboardButtonData("◀️ Отмена", "menu:people"),
			),
		)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "confirm_del_person":
		if len(parts) < 2 {
			return
		}
		personID := atoi(parts[1])
		if err := b.personService.Delete(personID, user.ID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}
		b.api.Request(tgbotapi.NewCallback(callback.ID, "🗑 Удалено!"))
		b.showPeople(chatID, msgID, user.ID)

	case "person":
		if len(parts) < 2 {
			return
		}
		personID := atoi(parts[1])
		person, _ := b.personService.Get(personID)
		if person == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Не найден"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		text := fmt.Sprintf("%s <b>%s</b>\n\nРоль: %s", person.RoleEmoji(), person.Name, person.RoleName())
		if person.HasBirthday() {
			text += fmt.Sprintf("\n🎂 %s", person.Birthday.Format("02.01.2006"))
			if person.Birthday.Year() > 1 {
				text += fmt.Sprintf(" (%d лет)", person.Age())
			}
			days := person.DaysUntilBirthday()
			if days == 0 {
				text += "\n<b>СЕГОДНЯ ДЕНЬ РОЖДЕНИЯ!</b>"
			} else {
				text += fmt.Sprintf("\nДо ДР: %d дн.", days)
			}
		}
		if person.Notes != "" {
			text += fmt.Sprintf("\n\n📝 %s", person.Notes)
		}

		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("del_person:%d", personID)),
				tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", "menu:people"),
			),
		)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "cl_check":
		// cl_check:checklistID:itemIndex
		if len(parts) < 3 {
			return
		}
		checklistID := atoi(parts[1])
		itemIndex := int(atoi(parts[2]))

		if err := b.checklistService.CheckItem(checklistID, user.ID, itemIndex); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "✅"))
		b.showChecklist(chatID, msgID, checklistID)

	case "cl_reset":
		// cl_reset:checklistID
		if len(parts) < 2 {
			return
		}
		checklistID := atoi(parts[1])

		if err := b.checklistService.Reset(checklistID, user.ID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "🔄 Сброшено"))
		b.showChecklist(chatID, msgID, checklistID)

	case "cl_del":
		// cl_del:checklistID - show confirm
		if len(parts) < 2 {
			return
		}
		checklistID := atoi(parts[1])
		c, _ := b.checklistService.Get(checklistID)
		if c == nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "Не найден"))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

		text := fmt.Sprintf("🗑 Удалить чек-лист <b>%s</b>?", c.Title)
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Да, удалить", fmt.Sprintf("cl_confirm_del:%d", checklistID)),
				tgbotapi.NewInlineKeyboardButtonData("◀️ Отмена", fmt.Sprintf("cl_view:%d", checklistID)),
			),
		)
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		edit.ParseMode = "HTML"
		edit.ReplyMarkup = &kb
		b.api.Send(edit)

	case "cl_confirm_del":
		// cl_confirm_del:checklistID
		if len(parts) < 2 {
			return
		}
		checklistID := atoi(parts[1])

		if err := b.checklistService.Delete(checklistID, user.ID); err != nil {
			b.api.Request(tgbotapi.NewCallback(callback.ID, "❌ "+err.Error()))
			return
		}

		b.api.Request(tgbotapi.NewCallback(callback.ID, "🗑 Удалено"))
		b.showChecklists(chatID, msgID, user.ID)

	case "cl_view":
		// cl_view:checklistID
		if len(parts) < 2 {
			return
		}
		checklistID := atoi(parts[1])
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		b.showChecklist(chatID, msgID, checklistID)

	case "add_checklist":
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		text := `<b>Создать чек-лист:</b>

/addchecklist Название
пункт 1
пункт 2
пункт 3

<b>Пример:</b>
/addchecklist Тим
Выспался ли он?
Поел ли нормально?
Какое настроение?`
		b.SendMessage(chatID, text)

	default:
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
	}
}

func (b *Bot) refreshTaskList(chatID int64, msgID int, userID int64) {
	b.showTaskListPage(chatID, msgID, 0, userID)
}

func (b *Bot) showTaskListPage(chatID int64, msgID int, page int, userID int64) {
	// Показываем задачи текущего чата
	tasks, _ := b.taskService.ListByChat(chatID, false)

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(userID)

	text := "<b>📋 Задачи</b>\n\n"
	if len(tasks) == 0 {
		text += "Нет активных задач 🎉\n\nНажми ➕ чтобы добавить"
	} else {
		text += b.taskService.FormatTaskListWithPersons(tasks, personNames)
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
	// Показываем срочные задачи текущего чата
	tasks, _ := b.taskService.ListForTodayByChat(chatID)

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(userID)

	text := "<b>📅 На сегодня</b>\n\n"
	if len(tasks) == 0 {
		text += "На сегодня задач нет! 🎉"
	} else {
		text += b.taskService.FormatTaskListWithPersons(tasks, personNames)
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

func (b *Bot) showPeople(chatID int64, msgID int, userID int64) {
	persons, _ := b.personService.List(userID)

	text := "<b>👥 Люди</b>\n\n"
	if len(persons) == 0 {
		text += "Список пуст.\n\nДобавь: /addperson Тим ребёнок 12.06.2017"
	} else {
		text += b.personService.FormatPersonList(persons)
	}

	kb := peopleKeyboard(persons)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showBirthdays(chatID int64, msgID int, userID int64) {
	persons, _ := b.personService.ListUpcomingBirthdays(userID, 60)

	text := "<b>🎂 Ближайшие дни рождения</b>\n\n"
	text += b.personService.FormatBirthdaysList(persons)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Все люди", "menu:people"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
		),
	)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showWeekSchedule(chatID int64, msgID int, userID int64) {
	events, _ := b.scheduleService.List(userID, true)

	text := "<b>📅 Недельное расписание</b>\n\n"
	text += b.scheduleService.FormatWeekSchedule(events)

	kb := weekScheduleKeyboard()

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showMainMenu(chatID int64, msgID int) {
	// Показываем статистику задач текущего чата
	tasks, _ := b.taskService.ListByChat(chatID, false)
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

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showShared(chatID int64, msgID int, userID int64) {
	tasks, _ := b.taskService.ListShared(false)

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(userID)

	text := "<b>👨‍👩‍👧 Общие задачи</b>\n\n"
	if len(tasks) == 0 {
		text += "Нет общих задач.\n\n💡 Сделай задачу общей: /share ID"
	} else {
		text += b.taskService.FormatTaskListWithPersons(tasks, personNames)
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Мои задачи", "menu:list"),
		),
	)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showFloating(chatID int64, msgID int, userID int64) {
	events, _ := b.scheduleService.ListFloating(userID)

	text := "<b>🔄 Плавающие события</b>\n\n"

	if len(events) == 0 {
		text += "Нет плавающих событий.\n\nДобавь: /addfloating Сб,Вс 10:00 Лука"
	} else {
		for _, e := range events {
			days := e.GetFloatingDays()
			var dayNames []string
			for _, d := range days {
				dayNames = append(dayNames, domain.WeekdayNameShort(d))
			}

			status := "❓ не выбран"
			if e.IsConfirmedThisWeek() && e.ConfirmedDay != nil {
				status = "✅ " + domain.WeekdayNameShort(domain.Weekday(*e.ConfirmedDay))
			}

			text += fmt.Sprintf("• <b>%s</b> %s\n  Дни: %s | %s\n\n",
				e.Title, e.TimeRange(),
				strings.Join(dayNames, "/"), status)
		}
	}

	kb := floatingListKeyboard(events)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showAutos(chatID int64, msgID int, userID int64) {
	autos, _ := b.autoService.List(userID)

	text := "<b>🚗 Мои машины</b>\n\n"
	text += b.autoService.FormatAutoList(autos)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add_auto"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
		),
	)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showChecklists(chatID int64, msgID int, userID int64) {
	checklists, _ := b.checklistService.List(userID)

	text := "<b>📋 Чек-листы</b>\n\n"
	if len(checklists) == 0 {
		text += "Нет чек-листов.\n\n/addchecklist — создать"
	} else {
		text += b.checklistService.FormatChecklistList(checklists)
	}

	kb := checklistsListKeyboard(checklists)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showChecklist(chatID int64, msgID int, checklistID int64) {
	c, _ := b.checklistService.Get(checklistID)
	if c == nil {
		return
	}

	text := b.checklistService.FormatChecklist(c)
	kb := checklistKeyboard(c)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showHistory(chatID int64, msgID int, userID int64) {
	tasks, _ := b.storage.ListCompletedTasks(userID, 20)

	text := "<b>📜 История выполненных задач</b>\n\n"
	if len(tasks) == 0 {
		text += "Пока нет выполненных задач"
	} else {
		for _, t := range tasks {
			doneDate := ""
			if t.DoneAt != nil {
				doneDate = t.DoneAt.Format("02.01")
			}
			text += fmt.Sprintf("✅ <b>#%d</b> %s <i>(%s)</i>\n", t.ID, t.Title, doneDate)
		}
		text += fmt.Sprintf("\n<i>Показано последних %d</i>", len(tasks))
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "menu:stats"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Активные", "menu:list"),
		),
	)

	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	edit.ReplyMarkup = &kb
	b.api.Send(edit)
}

func (b *Bot) showStats(chatID int64, msgID int, userID int64) {
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	monthAgo := now.AddDate(0, -1, 0)

	weekCompleted, weekCreated, _ := b.storage.GetTaskStats(userID, weekAgo)
	monthCompleted, monthCreated, _ := b.storage.GetTaskStats(userID, monthAgo)
	pendingCount, _ := b.storage.GetPendingTaskCount(userID)

	text := "<b>📊 Статистика задач</b>\n\n"
	text += fmt.Sprintf("<b>За неделю:</b>\n")
	text += fmt.Sprintf("  ✅ Выполнено: %d\n", weekCompleted)
	text += fmt.Sprintf("  ➕ Создано: %d\n\n", weekCreated)
	text += fmt.Sprintf("<b>За месяц:</b>\n")
	text += fmt.Sprintf("  ✅ Выполнено: %d\n", monthCompleted)
	text += fmt.Sprintf("  ➕ Создано: %d\n\n", monthCreated)
	text += fmt.Sprintf("<b>Сейчас активных:</b> %d", pendingCount)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📜 История", "menu:history"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
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
