package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
		b.cmdList(chatID, user, args)
	case "done":
		b.cmdDone(chatID, user, args)
	case "del":
		b.cmdDel(chatID, user, args)
	case "today":
		b.cmdToday(chatID, user)
	case "reminders":
		b.cmdReminders(chatID, user)
	case "menu":
		b.cmdMenu(chatID, user)
	case "people":
		b.cmdPeople(chatID, user)
	case "addperson":
		b.cmdAddPerson(chatID, user, args)
	case "birthdays":
		b.cmdBirthdays(chatID, user)
	case "week":
		b.cmdWeek(chatID, user, args)
	case "addweekly":
		b.cmdAddWeekly(chatID, user, args)
	case "delweekly":
		b.cmdDelWeekly(chatID, user, args)
	case "addfloating":
		b.cmdAddFloating(chatID, user, args)
	case "floating":
		b.cmdFloating(chatID, user)
	case "seedweek":
		b.cmdSeedWeek(chatID, user)
	case "seedpeople":
		b.cmdSeedPeople(chatID, user)
	case "assign":
		b.cmdAssign(chatID, user, args)
	case "shared":
		b.cmdShared(chatID, user)
	case "share":
		b.cmdShare(chatID, user, args)
	case "remind":
		b.cmdRemind(chatID, user, args)
	case "edit":
		b.cmdEdit(chatID, user, args)
	case "editreminder":
		b.cmdEditReminder(chatID, user, args)
	case "unshare":
		b.cmdUnshare(chatID, user, args)
	case "autos":
		b.cmdAutos(chatID, user)
	case "addauto":
		b.cmdAddAuto(chatID, user, args)
	case "insurance":
		b.cmdInsurance(chatID, user, args)
	case "maintenance":
		b.cmdMaintenance(chatID, user, args)
	case "seedautos":
		b.cmdSeedAutos(chatID, user)
	case "addrepeat":
		b.cmdAddRepeat(chatID, user, args)
	case "seedallnodes":
		b.cmdSeedAllnodes(chatID, user)
	case "checklist":
		b.cmdChecklist(chatID, user, args)
	case "checklists":
		b.cmdChecklists(chatID, user)
	case "addchecklist":
		b.cmdAddChecklist(chatID, user, args)
	case "delchecklist":
		b.cmdDelChecklist(chatID, user, args)
	case "seedchecklists":
		b.cmdSeedChecklists(chatID, user)
	case "history":
		b.cmdHistory(chatID, user)
	case "stats":
		b.cmdStats(chatID, user)
	case "linkperson":
		b.cmdLinkPerson(chatID, user, args)
	case "shareweekly":
		b.cmdShareWeekly(chatID, user, args)
	case "unshareweekly":
		b.cmdUnshareWeekly(chatID, user, args)
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
  <i>даты: завтра, 20 января, 04.02, в пятницу</i>
/list — список задач
/done ID — выполнить задачу
/del ID — удалить задачу
/today — задачи на сегодня
/remind ID 1д,1ч — напоминание до дедлайна
/assign ID кому — назначить задачу
/shared — общие семейные задачи
/share ID — сделать задачу общей

<b>Расписание</b>
/week — недельное расписание
/addweekly Пн 17:30 Событие
/addfloating Сб,Вс 10:00 Лука
/floating — плавающие события

<b>Люди</b>
/people — список людей
/addperson Имя роль ДД.ММ.ГГГГ
/birthdays — ближайшие ДР

<b>Чек-листы</b>
/checklist Название — показать чек-лист
/checklists — все чек-листы
/addchecklist — создать чек-лист

<b>Напоминания</b>
/reminders — список напоминаний

<b>Статистика</b>
/history — выполненные задачи
/stats — статистика за неделю/месяц

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

	// Парсим @упоминания и извлекаем чистый текст
	cleanText, mentions := b.taskService.ParseMentions(args)

	// Резолвим @mention через гибридный поиск (People -> Users)
	var personID *int64
	var assignedTo *int64
	var personName string
	for _, mention := range mentions {
		resolved, err := b.taskService.ResolveMention(user.ID, mention)
		if err == nil && resolved != nil {
			personID = resolved.PersonID
			assignedTo = resolved.UserID
			personName = resolved.Name
			break // Берём первое найденное упоминание
		}
	}

	args = cleanText

	// Парсим дату из текста (завтра, в понедельник, через неделю)
	args, dueDate := b.taskService.ParseDate(args)

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
		hint := "Выбери приоритет:\n\n<b>" + args + "</b>"
		if personName != "" {
			hint += fmt.Sprintf("\n\n👤 Для: %s", personName)
		}
		if dueDate != nil {
			hint += fmt.Sprintf("\n📅 %s", dueDate.Format("02.01.2006"))
		}
		b.SendMessageWithKeyboard(chatID, hint, kb)
		return
	}

	task, err := b.taskService.CreateFull(user.ID, chatID, args, priority, personID, dueDate)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	// Если есть связь с Telegram — назначаем пользователю
	if assignedTo != nil {
		_ = b.taskService.Assign(task.ID, *assignedTo, user.ID, chatID)
	}

	text := fmt.Sprintf("✅ Задача добавлена\n\n%s <b>#%d</b> %s", task.PriorityEmoji(), task.ID, task.Title)
	if personName != "" {
		text += fmt.Sprintf("\n👤 @%s", personName)
	}
	if task.DueDate != nil {
		text += fmt.Sprintf("\n📅 %s", task.DueDate.Format("02.01.2006"))
	}
	kb := taskKeyboard(task.ID)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdList(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	var tasks []*domain.Task
	var err error
	var filterName string

	// Проверяем фильтр по @тегу
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "@") {
		personName := strings.TrimPrefix(args, "@")
		person, _ := b.personService.GetByName(user.ID, personName)
		if person != nil {
			tasks, err = b.taskService.ListByPerson(person.ID, false)
			filterName = person.Name
		} else {
			b.SendMessage(chatID, "❌ Человек не найден: @"+personName)
			return
		}
	} else {
		// Показываем задачи текущего чата
		tasks, err = b.taskService.ListByChat(chatID, false)
	}

	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(user.ID)

	text := "<b>📋 Задачи</b>"
	if filterName != "" {
		text += fmt.Sprintf(" <i>(@%s)</i>", filterName)
	}
	text += "\n\n"

	if len(tasks) == 0 {
		if filterName != "" {
			text += fmt.Sprintf("У %s нет активных задач", filterName)
		} else {
			text += "Нет активных задач 🎉\n\nНажми ➕ чтобы добавить"
		}
	} else {
		text += b.taskService.FormatTaskListWithPersons(tasks, personNames)
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

	if err := b.taskService.MarkDone(taskID, user.ID, chatID); err != nil {
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

func (b *Bot) cmdDel(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID задачи: /del 5")
		return
	}

	taskID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	// Get task to show what's being deleted
	task, err := b.storage.GetTask(taskID)
	if err != nil || task == nil {
		b.SendMessage(chatID, "Задача не найдена")
		return
	}

	if err := b.taskService.Delete(taskID, user.ID, chatID); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("🗑 Задача <b>#%d</b> удалена:\n<s>%s</s>", taskID, task.Title)
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

	// Показываем срочные задачи текущего чата
	tasks, err := b.taskService.ListForTodayByChat(chatID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(user.ID)

	text := "<b>📅 На сегодня</b>\n\n"
	if len(tasks) == 0 {
		text += "На сегодня задач нет! 🎉"
	} else {
		text += b.taskService.FormatTaskListWithPersons(tasks, personNames)
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

func (b *Bot) cmdPeople(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	persons, err := b.personService.List(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>👥 Люди</b>\n\n"
	if len(persons) == 0 {
		text += "Список пуст.\n\nДобавь: /addperson Тим ребёнок 12.06.2017"
	} else {
		text += b.personService.FormatPersonList(persons)
	}

	kb := peopleKeyboard(persons)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdAddPerson(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Добавить человека:</b>

/addperson Имя роль ДД.ММ.ГГГГ

<b>Примеры:</b>
/addperson Тим ребёнок 12.06.2017
/addperson Ира семья 17.12
/addperson Федя контакт

<b>Роли:</b> ребёнок, семья, контакт`
		b.SendMessage(chatID, text)
		return
	}

	name, role, birthday, err := b.personService.ParseAddPersonArgs(args)
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	person, err := b.personService.Create(user.ID, name, role, birthday, "")
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Добавлен: %s <b>%s</b>", person.RoleEmoji(), person.Name)
	if person.HasBirthday() {
		text += fmt.Sprintf("\n🎂 %s", person.Birthday.Format("02.01.2006"))
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 К списку", "menu:people"),
			tgbotapi.NewInlineKeyboardButtonData("🎂 Дни рождения", "menu:birthdays"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdBirthdays(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	// Показываем ДР на ближайшие 60 дней
	persons, err := b.personService.ListUpcomingBirthdays(user.ID, 60)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>🎂 Ближайшие дни рождения</b>\n\n"
	text += b.personService.FormatBirthdaysList(persons)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Все люди", "menu:people"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Задачи", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdWeek(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	// Include shared events so family members can see each other's schedule
	events, err := b.scheduleService.List(user.ID, true)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	showIDs := strings.ToLower(strings.TrimSpace(args)) == "ids"

	text := "<b>📅 Недельное расписание</b>\n\n"
	if showIDs {
		text += b.scheduleService.FormatWeekScheduleWithIDs(events)
		text += "\n💡 /shareweekly ID — сделать общим"
	} else {
		text += b.scheduleService.FormatWeekSchedule(events)
	}

	kb := weekScheduleKeyboard()
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdAddWeekly(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Добавить регулярное событие:</b>

/addweekly День Время Название
/addweekly День Время !N Название

<b>Примеры:</b>
/addweekly Пн 17:30 Федя спорт
/addweekly Ср 16:00-20:00 Тим плавание
/addweekly Сб 10:00 !15 Шахматы

<b>!N</b> — напомнить за N минут
<b>Дни:</b> Пн, Вт, Ср, Чт, Пт, Сб, Вс`
		b.SendMessage(chatID, text)
		return
	}

	dayOfWeek, timeStart, timeEnd, title, reminderBefore, err := b.scheduleService.ParseAddArgs(args)
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	event, err := b.scheduleService.Create(user.ID, dayOfWeek, timeStart, timeEnd, title, reminderBefore)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	timeStr := event.TimeRange()
	text := fmt.Sprintf("✅ Добавлено: %s <b>%s</b> %s — %s",
		domain.WeekdayEmoji(event.DayOfWeek),
		event.DayName(),
		timeStr,
		event.Title)

	if event.ReminderBefore > 0 {
		text += fmt.Sprintf("\n🔔 Напомню за %d мин", event.ReminderBefore)
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdDelWeekly(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID события: /delweekly 1")
		return
	}

	eventID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID события")
		return
	}

	if err := b.scheduleService.Delete(eventID, user.ID); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "✅ Событие удалено"
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdAddFloating(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Добавить плавающее событие:</b>

/addfloating Дни Время Название

Плавающее событие — это событие которое происходит в один из дней (выбираешь каждую неделю).

<b>Примеры:</b>
/addfloating Сб,Вс 10:00 Лука
/addfloating Пт,Сб 19:00 Кино

<b>Формат:</b> Дни через запятую (мин. 2)`
		b.SendMessage(chatID, text)
		return
	}

	days, timeStart, timeEnd, title, err := b.scheduleService.ParseFloatingArgs(args)
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	event, err := b.scheduleService.CreateFloating(user.ID, days, timeStart, timeEnd, title)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	var dayNames []string
	for _, d := range days {
		dayNames = append(dayNames, domain.WeekdayNameShort(d))
	}

	text := fmt.Sprintf("✅ Добавлено плавающее: 🔄 <b>%s</b> %s (%s)",
		event.Title,
		event.TimeRange(),
		strings.Join(dayNames, "/"))

	kb := floatingEventKeyboard(event)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdFloating(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	events, err := b.scheduleService.ListFloating(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	if len(events) == 0 {
		text := "Нет плавающих событий.\n\nДобавь: /addfloating Сб,Вс 10:00 Лука"
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
			),
		)
		b.SendMessageWithKeyboard(chatID, text, kb)
		return
	}

	text := "<b>🔄 Плавающие события</b>\n\n"
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

		sharedMark := ""
		if e.IsShared {
			sharedMark = " 👨‍👩‍👧‍👦"
		}

		text += fmt.Sprintf("<code>#%d</code> <b>%s</b> %s%s\n  Дни: %s | %s\n\n",
			e.ID, e.Title, e.TimeRange(), sharedMark,
			strings.Join(dayNames, "/"), status)
	}
	text += "💡 /shareweekly ID — сделать общим"

	kb := floatingListKeyboard(events)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// cmdSeedWeek seeds the default weekly schedule
func (b *Bot) cmdSeedWeek(chatID int64, user *domain.User) {
	// Check if user is owner
	if user.TelegramID != b.cfg.OwnerTelegramID {
		b.SendMessage(chatID, "❌ Только владелец может заполнить расписание")
		return
	}

	// Default schedule based on TODO.md
	regularEvents := []struct {
		dayOfWeek    domain.Weekday
		timeStart    string
		timeEnd      string
		title        string
		reminderMins int
	}{
		{domain.WeekdayMonday, "16:00", "", "Психолог", 30},
		{domain.WeekdayMonday, "17:30", "", "Федя на спорт", 15},
		{domain.WeekdayTuesday, "09:00", "", "Дежурство Allnodes", 60},
		{domain.WeekdayWednesday, "16:00", "20:00", "Тимофей", 30},
		{domain.WeekdayThursday, "15:00", "18:00", "Созвон Allnodes", 15},
		{domain.WeekdayFriday, "15:00", "", "Выезд к психологу Тима", 60},
		{domain.WeekdaySaturday, "10:00", "20:00", "Тимофей", 30},
	}

	// Floating events
	floatingEvents := []struct {
		days      []domain.Weekday
		timeStart string
		timeEnd   string
		title     string
	}{
		{[]domain.Weekday{domain.WeekdaySaturday, domain.WeekdaySunday}, "12:00", "21:00", "Лука"},
	}

	created := 0

	// Create regular events
	for _, e := range regularEvents {
		_, err := b.scheduleService.Create(user.ID, e.dayOfWeek, e.timeStart, e.timeEnd, e.title, e.reminderMins)
		if err != nil {
			// Skip duplicates or errors
			continue
		}
		created++
	}

	// Create floating events
	for _, e := range floatingEvents {
		_, err := b.scheduleService.CreateFloating(user.ID, e.days, e.timeStart, e.timeEnd, e.title)
		if err != nil {
			// Skip duplicates or errors
			continue
		}
		created++
	}

	if created == 0 {
		b.SendMessage(chatID, "Расписание уже заполнено или произошла ошибка")
		return
	}

	b.SendMessage(chatID, fmt.Sprintf("✅ Добавлено %d событий в расписание\n\n/week — посмотреть", created))
}

// cmdSeedPeople seeds the default people with birthdays
func (b *Bot) cmdSeedPeople(chatID int64, user *domain.User) {
	// Check if user is owner
	if user.TelegramID != b.cfg.OwnerTelegramID {
		b.SendMessage(chatID, "❌ Только владелец может добавить людей")
		return
	}

	// Default people based on TODO.md
	people := []struct {
		name     string
		role     domain.PersonRole
		birthday string // DD.MM.YYYY or DD.MM
	}{
		{"Тим", domain.RolePartnerChild, "12.06.2017"},
		{"Лука", domain.RoleChild, "18.09.2021"},
		{"Ира", domain.RoleFamily, "17.12"},
		{"Федя", domain.RoleChild, "23.09"},
	}

	created := 0
	for _, p := range people {
		// Parse birthday using the service's parseDate method logic
		var birthday *time.Time
		if p.birthday != "" {
			// Parse date in DD.MM.YYYY or DD.MM format
			_, _, parsedBD, _ := b.personService.ParseAddPersonArgs(p.name + " _ " + p.birthday)
			birthday = parsedBD
		}

		_, err := b.personService.Create(user.ID, p.name, p.role, birthday, "")
		if err != nil {
			// Skip duplicates or errors
			continue
		}
		created++
	}

	if created == 0 {
		b.SendMessage(chatID, "Люди уже добавлены или произошла ошибка")
		return
	}

	b.SendMessage(chatID, fmt.Sprintf("✅ Добавлено %d человек\n\n/people — посмотреть\n/birthdays — дни рождения", created))
}

func (b *Bot) cmdAssign(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Назначить задачу:</b>

/assign ID мне — назначить себе
/assign ID @имя — назначить человеку/пользователю

<b>Примеры:</b>
/assign 5 мне
/assign 12 @ира
/assign 17 @тим

💡 Сначала ищется в /people, потом в Telegram`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Укажи: /assign ID кому")
		return
	}

	taskID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	// Получаем задачу
	task, _ := b.storage.GetTask(taskID)
	if task == nil {
		b.SendMessage(chatID, "❌ Задача не найдена")
		return
	}

	target := strings.ToLower(parts[1])

	var assignToUserID *int64
	var personID *int64
	var assignedName string
	var notifyTelegramID *int64

	switch {
	case target == "мне" || target == "me" || target == "себе":
		// Назначить себе
		assignToUserID = &user.ID
		assignedName = "тебе"
	case strings.HasPrefix(target, "@"):
		// Гибридный поиск: сначала People, потом Users
		mention := strings.TrimPrefix(target, "@")
		resolved, err := b.taskService.ResolveMention(user.ID, mention)
		if err != nil {
			b.SendMessage(chatID, "❌ Не найдено: "+target+"\n\n💡 Добавь через /addperson или /linkperson")
			return
		}

		assignedName = resolved.Name
		personID = resolved.PersonID
		assignToUserID = resolved.UserID
		notifyTelegramID = resolved.TelegramID
	default:
		b.SendMessage(chatID, "❌ Неизвестный формат. Используй: мне, @имя")
		return
	}

	// Обновляем PersonID (если есть)
	if personID != nil {
		_ = b.taskService.LinkToPerson(taskID, user.ID, personID)
	}

	// Обновляем AssignedTo (если есть связь с Telegram)
	if assignToUserID != nil {
		if err := b.taskService.Assign(taskID, *assignToUserID, user.ID, chatID); err != nil {
			b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
			return
		}
	}

	// Формируем ответ
	var statusText string
	if assignToUserID != nil {
		statusText = fmt.Sprintf("✅ Задача <b>#%d</b> назначена %s", taskID, assignedName)
	} else {
		// Person без Telegram
		statusText = fmt.Sprintf("✅ Задача <b>#%d</b> помечена для @%s", taskID, assignedName)
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, statusText, kb)

	// Отправляем уведомление (если есть Telegram и это не сам себе)
	if notifyTelegramID != nil && (assignToUserID == nil || *assignToUserID != user.ID) {
		notifyText := fmt.Sprintf("📬 <b>%s</b> назначил тебе задачу:\n\n%s <b>#%d</b> %s",
			user.Name, task.PriorityEmoji(), task.ID, task.Title)
		notifyKb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Выполнить", fmt.Sprintf("done:%d", taskID)),
				tgbotapi.NewInlineKeyboardButtonData("📋 Все задачи", "menu:list"),
			),
		)
		b.SendMessageWithKeyboard(*notifyTelegramID, notifyText, notifyKb)
	}
}

func (b *Bot) cmdShared(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks, err := b.taskService.ListShared(false)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	// Получаем имена людей для отображения
	personNames, _ := b.personService.GetNamesMap(user.ID)

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
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdShare(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Сделать задачу общей:</b>

/share ID — сделать задачу общей для семьи
/unshare ID — убрать из общих

<b>Пример:</b>
/share 5`
		b.SendMessage(chatID, text)
		return
	}

	taskID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	if err := b.taskService.SetShared(taskID, user.ID, chatID, true); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Задача <b>#%d</b> теперь общая", taskID)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👨‍👩‍👧 Общие", "menu:shared"),
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdUnshare(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID задачи: /unshare 5")
		return
	}

	taskID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	if err := b.taskService.SetShared(taskID, user.ID, chatID, false); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Задача <b>#%d</b> больше не общая", taskID)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdRemind(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Добавить напоминание к задаче:</b>

/remind ID интервалы

<b>Интервалы:</b>
• неделя, нед, 1н — за неделю
• день, 1д — за день
• 3ч — за 3 часа
• час, 1ч — за час
• 30м — за 30 минут

<b>Примеры:</b>
/remind 5 1д,1ч — за день и за час
/remind 5 неделя,день,час`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Укажи ID задачи и интервалы: /remind 5 1д,1ч")
		return
	}

	taskID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	// Check task exists and has due_date
	task, err := b.storage.GetTask(taskID)
	if err != nil || task == nil {
		b.SendMessage(chatID, "Задача не найдена")
		return
	}

	if task.DueDate == nil {
		b.SendMessage(chatID, "❌ У задачи нет даты. Добавь дату: /add текст завтра")
		return
	}

	// Parse intervals
	intervalsStr := strings.Join(parts[1:], ",")
	intervals := strings.Split(intervalsStr, ",")

	var added []string
	for _, intStr := range intervals {
		minutes, ok := domain.ParseRemindInterval(intStr)
		if !ok {
			continue
		}

		tr := &domain.TaskReminder{
			TaskID:       taskID,
			RemindBefore: minutes,
		}
		if err := b.storage.CreateTaskReminder(tr); err != nil {
			continue
		}
		added = append(added, domain.RemindBeforeLabel(minutes))
	}

	if len(added) == 0 {
		b.SendMessage(chatID, "❌ Не удалось добавить напоминания. Проверь интервалы.")
		return
	}

	text := fmt.Sprintf("✅ Напоминания для <b>#%d</b>:\n%s\n\n📅 Дедлайн: %s",
		taskID,
		strings.Join(added, ", "),
		task.DueDate.Format("02.01.2006 15:04"))

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К задачам", "menu:list"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdEdit(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Редактировать задачу:</b>

/edit ID — показать задачу и опции
/edit ID текст Новый текст
/edit ID приоритет срочно|неделя|потом
/edit ID дата завтра|20.01|20 января

<b>Примеры:</b>
/edit 5
/edit 5 текст Позвонить врачу
/edit 5 приоритет срочно
/edit 5 дата завтра`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.SplitN(args, " ", 3)
	taskID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID задачи")
		return
	}

	task, err := b.taskService.Get(taskID)
	if err != nil || task == nil {
		b.SendMessage(chatID, "Задача не найдена")
		return
	}

	// Just /edit ID — show task info with edit buttons
	if len(parts) == 1 {
		dueStr := "не установлена"
		if task.DueDate != nil {
			dueStr = task.DueDate.Format("02.01.2006")
		}
		text := fmt.Sprintf("<b>✏️ Редактирование #%d</b>\n\n%s <b>%s</b>\n📅 Дата: %s\n🎯 Приоритет: %s",
			task.ID, task.PriorityEmoji(), task.Title, dueStr, priorityName(task.Priority))

		kb := editTaskKeyboard(task.ID)
		b.SendMessageWithKeyboard(chatID, text, kb)
		return
	}

	if len(parts) < 3 {
		b.SendMessage(chatID, "Укажи поле и значение: /edit ID поле значение")
		return
	}

	field := strings.ToLower(parts[1])
	value := parts[2]

	switch field {
	case "текст", "title", "название":
		if err := b.taskService.UpdateTitle(taskID, user.ID, chatID, value); err != nil {
			b.SendMessage(chatID, "❌ "+err.Error())
			return
		}
		b.SendMessage(chatID, fmt.Sprintf("✅ Текст задачи #%d обновлён", taskID))

	case "приоритет", "priority", "pri":
		var priority domain.Priority
		switch strings.ToLower(value) {
		case "срочно", "urgent", "1":
			priority = domain.PriorityUrgent
		case "неделя", "week", "2":
			priority = domain.PriorityWeek
		case "потом", "someday", "3":
			priority = domain.PrioritySomeday
		default:
			b.SendMessage(chatID, "Неверный приоритет. Доступно: срочно, неделя, потом")
			return
		}
		if err := b.taskService.UpdatePriority(taskID, user.ID, chatID, priority); err != nil {
			b.SendMessage(chatID, "❌ "+err.Error())
			return
		}
		b.SendMessage(chatID, fmt.Sprintf("✅ Приоритет задачи #%d: %s", taskID, priorityName(priority)))

	case "дата", "date", "due":
		_, dueDate := b.taskService.ParseDate(value)
		if dueDate == nil {
			// Try parsing as DD.MM.YYYY directly
			t, err := time.Parse("02.01.2006", value)
			if err == nil {
				dueDate = &t
			}
		}
		if err := b.taskService.UpdateDueDate(taskID, user.ID, chatID, dueDate); err != nil {
			b.SendMessage(chatID, "❌ "+err.Error())
			return
		}
		dateStr := "убрана"
		if dueDate != nil {
			dateStr = dueDate.Format("02.01.2006")
		}
		b.SendMessage(chatID, fmt.Sprintf("✅ Дата задачи #%d: %s", taskID, dateStr))

	default:
		b.SendMessage(chatID, "Неизвестное поле. Доступно: текст, приоритет, дата")
	}
}

func priorityName(p domain.Priority) string {
	switch p {
	case domain.PriorityUrgent:
		return "🔴 срочно"
	case domain.PriorityWeek:
		return "🟡 неделя"
	case domain.PrioritySomeday:
		return "🟢 потом"
	default:
		return "⚪ не задан"
	}
}

func (b *Bot) cmdEditReminder(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Редактировать напоминание:</b>

/editreminder ID — показать напоминание
/editreminder ID текст Новый текст
/editreminder ID время 09:30

<b>Примеры:</b>
/editreminder 5
/editreminder 5 текст Напомнить о встрече
/editreminder 5 время 10:00`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.SplitN(args, " ", 3)
	reminderID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID напоминания")
		return
	}

	reminder, err := b.reminderService.Get(reminderID)
	if err != nil || reminder == nil {
		b.SendMessage(chatID, "Напоминание не найдено")
		return
	}

	if reminder.UserID != user.ID {
		b.SendMessage(chatID, "Нет доступа к этому напоминанию")
		return
	}

	// Just /editreminder ID — show reminder info
	if len(parts) == 1 {
		nextRun := "не установлено"
		if reminder.NextRun != nil {
			nextRun = reminder.NextRun.Format("02.01.2006 15:04")
		}
		text := fmt.Sprintf("<b>✏️ Напоминание #%d</b>\n\n📝 %s\n⏰ Следующий запуск: %s\n🔄 Тип: %s\n📅 Расписание: %s",
			reminder.ID, reminder.Title, nextRun, reminder.Type, reminder.Schedule)
		b.SendMessage(chatID, text)
		return
	}

	if len(parts) < 3 {
		b.SendMessage(chatID, "Укажи поле и значение: /editreminder ID поле значение")
		return
	}

	field := strings.ToLower(parts[1])
	value := parts[2]

	switch field {
	case "текст", "title", "название":
		if err := b.storage.UpdateReminderTitle(reminderID, value); err != nil {
			b.SendMessage(chatID, "❌ "+err.Error())
			return
		}
		b.SendMessage(chatID, fmt.Sprintf("✅ Текст напоминания #%d обновлён", reminderID))

	case "время", "time":
		// Parse time and update next_run
		t, err := time.Parse("15:04", value)
		if err != nil {
			b.SendMessage(chatID, "Неверный формат времени. Используй ЧЧ:ММ (например 09:30)")
			return
		}
		now := time.Now()
		newNextRun := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if newNextRun.Before(now) {
			newNextRun = newNextRun.AddDate(0, 0, 1)
		}
		reminder.NextRun = &newNextRun
		if err := b.storage.UpdateReminder(reminder); err != nil {
			b.SendMessage(chatID, "❌ "+err.Error())
			return
		}
		b.SendMessage(chatID, fmt.Sprintf("✅ Время напоминания #%d: %s", reminderID, newNextRun.Format("02.01.2006 15:04")))

	default:
		b.SendMessage(chatID, "Неизвестное поле. Доступно: текст, время")
	}
}

func (b *Bot) cmdAutos(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	autos, err := b.autoService.List(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>🚗 Машины</b>\n\n"
	text += b.autoService.FormatAutoList(autos)

	if len(autos) == 0 {
		text += "\n/addauto Название год — добавить машину"
		text += "\n/seedautos — добавить дефолтные"
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", "add_auto"),
			tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu:main"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdAddAuto(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Добавить машину:</b>

/addauto Название год

<b>Примеры:</b>
/addauto Ford Raptor 2014
/addauto Lexus RX 2015
/addauto Peugeot 4008 2012`
		b.SendMessage(chatID, text)
		return
	}

	name, year, err := b.autoService.ParseAddArgs(args)
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	auto, err := b.autoService.Create(user.ID, name, year)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	yearStr := ""
	if auto.Year > 0 {
		yearStr = fmt.Sprintf(" (%d)", auto.Year)
	}
	text := fmt.Sprintf("✅ Машина добавлена: 🚗 <b>#%d</b> %s%s", auto.ID, auto.Name, yearStr)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚗 Все машины", "menu:autos"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdInsurance(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Установить дату страховки:</b>

/insurance ID ДД.ММ.ГГГГ

<b>Примеры:</b>
/insurance 1 15.06.2025
/insurance 2 01.12`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Укажи ID и дату: /insurance 1 15.06.2025")
		return
	}

	autoID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID")
		return
	}

	date, err := b.autoService.ParseDate(parts[1])
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	if err := b.autoService.SetInsurance(autoID, user.ID, date); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Страховка установлена до %s", date.Format("02.01.2006"))
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚗 Все машины", "menu:autos"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdMaintenance(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Установить дату ТО:</b>

/maintenance ID ДД.ММ.ГГГГ

<b>Примеры:</b>
/maintenance 1 15.06.2025
/maintenance 2 01.12`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Укажи ID и дату: /maintenance 1 15.06.2025")
		return
	}

	autoID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID")
		return
	}

	date, err := b.autoService.ParseDate(parts[1])
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	if err := b.autoService.SetMaintenance(autoID, user.ID, date); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ ТО установлено до %s", date.Format("02.01.2006"))
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚗 Все машины", "menu:autos"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

func (b *Bot) cmdSeedAutos(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	autos := []struct {
		name string
		year int
	}{
		{"Ford F-150 Raptor", 2014},
		{"Lexus RX", 2015},
		{"Peugeot 4008", 2012},
	}

	created := 0
	for _, a := range autos {
		if _, err := b.autoService.Create(user.ID, a.name, a.year); err == nil {
			created++
		}
	}

	text := fmt.Sprintf("✅ Добавлено машин: %d", created)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚗 Все машины", "menu:autos"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdAddRepeat creates a repeating task
// Usage: /addrepeat daily|weekdays|weekly|monthly|monthly_nth HH:MM Название задачи
func (b *Bot) cmdAddRepeat(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Создать повторяющуюся задачу:</b>

/addrepeat ТИП ЧЧ:ММ Название

<b>Типы:</b>
• daily — каждый день
• weekdays — Пн-Пт
• weekly — раз в неделю
• monthly ДЕНЬ — N-е число каждого месяца
• monthly_nth N День — N-я неделя месяца

<b>Примеры:</b>
/addrepeat daily 09:15 Утренний статус
/addrepeat weekdays 09:00 Дейли-статус
/addrepeat monthly 4 11:00 Отчёт Apostol
/addrepeat monthly_nth 2 Пт 09:00 Дежурство`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Формат: /addrepeat ТИП ЧЧ:ММ Название")
		return
	}

	repeatTypeStr := strings.ToLower(parts[0])

	var repeatType domain.RepeatType
	var timeStr, title string
	var weekNum int
	var weekday time.Weekday

	// Handle monthly specially: /addrepeat monthly 4 11:00 Отчёт
	if repeatTypeStr == "monthly" {
		monthlyParts := strings.SplitN(args, " ", 4)
		if len(monthlyParts) < 4 {
			b.SendMessage(chatID, "Формат: /addrepeat monthly ДЕНЬ ЧЧ:ММ Название\nПример: /addrepeat monthly 4 11:00 Отчёт Apostol")
			return
		}

		// Parse day of month
		dayOfMonth, err := strconv.Atoi(monthlyParts[1])
		if err != nil || dayOfMonth < 1 || dayOfMonth > 31 {
			b.SendMessage(chatID, "Неверный день месяца (должен быть 1-31)")
			return
		}
		weekNum = dayOfMonth // Reuse weekNum to store day of month

		timeStr = monthlyParts[2]
		title = monthlyParts[3]
		repeatType = domain.RepeatMonthly
	} else if repeatTypeStr == "monthly_nth" {
		// Handle monthly_nth specially: /addrepeat monthly_nth 2 Пт 09:00 Дежурство
		monthlyParts := strings.SplitN(args, " ", 5)
		if len(monthlyParts) < 5 {
			b.SendMessage(chatID, "Формат: /addrepeat monthly_nth НЕДЕЛЯ ДЕНЬ ЧЧ:ММ Название\nПример: /addrepeat monthly_nth 2 Пт 09:00 Дежурство")
			return
		}

		// Parse week number
		weekNumParsed, err := strconv.Atoi(monthlyParts[1])
		if err != nil || weekNumParsed < 1 || weekNumParsed > 4 {
			b.SendMessage(chatID, "Неверный номер недели (должен быть 1-4)")
			return
		}
		weekNum = weekNumParsed

		// Parse weekday
		dayParsed, err := domain.ParseWeekdayShort(monthlyParts[2])
		if err != nil {
			b.SendMessage(chatID, "Неверный день недели: "+monthlyParts[2]+"\nДоступны: Пн, Вт, Ср, Чт, Пт, Сб, Вс")
			return
		}
		weekday = dayParsed

		timeStr = monthlyParts[3]
		title = monthlyParts[4]
		repeatType = domain.RepeatMonthlyNth
	} else {
		if len(parts) < 3 {
			b.SendMessage(chatID, "Формат: /addrepeat ТИП ЧЧ:ММ Название")
			return
		}
		timeStr = parts[1]
		title = parts[2]

		switch repeatTypeStr {
		case "daily":
			repeatType = domain.RepeatDaily
		case "weekdays":
			repeatType = domain.RepeatWeekdays
		case "weekly":
			repeatType = domain.RepeatWeekly
		default:
			b.SendMessage(chatID, "Неизвестный тип: "+repeatTypeStr+"\nДоступны: daily, weekdays, weekly, monthly, monthly_nth")
			return
		}
	}

	// Validate time format
	if _, err := time.Parse("15:04", timeStr); err != nil {
		b.SendMessage(chatID, "Неверный формат времени. Используй ЧЧ:ММ (например 09:15)")
		return
	}

	// Create the first task with due date
	now := time.Now()
	var dueDate *time.Time

	switch repeatType {
	case domain.RepeatWeekdays:
		// Skip to Monday if it's weekend
		next := now
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
		dueDate = &next
	case domain.RepeatMonthly:
		// Find next day of month (weekNum stores day of month for monthly)
		dayOfMonth := weekNum
		next := time.Date(now.Year(), now.Month(), dayOfMonth, 0, 0, 0, 0, now.Location())
		if next.Before(now) || next.Equal(now) {
			// This month's already passed, go to next month
			next = time.Date(now.Year(), now.Month()+1, dayOfMonth, 0, 0, 0, 0, now.Location())
		}
		dueDate = &next
	case domain.RepeatMonthlyNth:
		// Find next Nth weekday
		next := domain.NthWeekdayOfMonth(now.Year(), now.Month(), weekday, weekNum)
		if next.Before(now) {
			// This month's already passed, go to next month
			nextMonth := now.AddDate(0, 1, 0)
			next = domain.NthWeekdayOfMonth(nextMonth.Year(), nextMonth.Month(), weekday, weekNum)
		}
		dueDate = &next
	default:
		dueDate = &now
	}

	task, err := b.taskService.CreateRepeatingWithWeekNum(
		user.ID,
		chatID,
		title,
		domain.PriorityUrgent,
		nil,
		dueDate,
		repeatType,
		timeStr,
		weekNum,
	)
	if err != nil {
		b.SendMessage(chatID, "❌ "+err.Error())
		return
	}

	repeatNames := map[domain.RepeatType]string{
		domain.RepeatDaily:      "ежедневно",
		domain.RepeatWeekdays:   "Пн-Пт",
		domain.RepeatWeekly:     "еженедельно",
		domain.RepeatMonthly:    "ежемесячно",
		domain.RepeatMonthlyNth: fmt.Sprintf("%d-я неделя", weekNum),
	}

	text := fmt.Sprintf("✅ Создана повторяющаяся задача\n\n🔁 <b>#%d</b> %s\n⏰ %s (%s)",
		task.ID, task.Title, timeStr, repeatNames[repeatType])
	b.SendMessage(chatID, text)
}

// cmdSeedAllnodes creates Allnodes status tasks
func (b *Bot) cmdSeedAllnodes(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks := []struct {
		title      string
		repeatTime string
	}{
		{"Утренний статус Allnodes", "09:15"},
		{"Вечерний статус Allnodes", "18:00"},
	}

	created := 0
	for _, t := range tasks {
		now := time.Now()
		// Skip to next weekday if weekend
		for now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
			now = now.AddDate(0, 0, 1)
		}

		_, err := b.taskService.CreateRepeating(
			user.ID,
			chatID,
			t.title,
			domain.PriorityUrgent,
			nil,
			&now,
			domain.RepeatWeekdays,
			t.repeatTime,
		)
		if err == nil {
			created++
		}
	}

	text := fmt.Sprintf("✅ Создано задач Allnodes: %d\n\n🔁 Утренний статус — 09:15 (Пн-Пт)\n🔁 Вечерний статус — 18:00 (Пн-Пт)", created)
	b.SendMessage(chatID, text)
}

// cmdChecklist shows a checklist by name
func (b *Bot) cmdChecklist(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Показать чек-лист:</b>

/checklist Название

<b>Примеры:</b>
/checklist Тим
/checklist Перед поездкой

<b>Список чек-листов:</b> /checklists`
		b.SendMessage(chatID, text)
		return
	}

	c, err := b.checklistService.GetByTitle(user.ID, args)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}
	if c == nil {
		b.SendMessage(chatID, "❌ Чек-лист не найден: "+args)
		return
	}

	text := b.checklistService.FormatChecklist(c)
	kb := checklistKeyboard(c)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdChecklists shows all checklists
func (b *Bot) cmdChecklists(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	checklists, err := b.checklistService.List(user.ID)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "<b>📋 Чек-листы</b>\n\n"
	if len(checklists) == 0 {
		text += "Нет чек-листов.\n\n/addchecklist — создать"
	} else {
		text += b.checklistService.FormatChecklistList(checklists)
	}

	kb := checklistsListKeyboard(checklists)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdAddChecklist creates a new checklist
func (b *Bot) cmdAddChecklist(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Создать чек-лист:</b>

/addchecklist Название
пункт 1
пункт 2
пункт 3

<b>Пример:</b>
/addchecklist Тим
Выспался ли он?
Поел ли нормально?
Какое настроение?
Что говорит психолог?`
		b.SendMessage(chatID, text)
		return
	}

	// Parse: first line is title, rest are items
	lines := strings.Split(args, "\n")
	title := strings.TrimSpace(lines[0])

	var items []string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line != "" {
			items = append(items, line)
		}
	}

	if len(items) == 0 {
		b.SendMessage(chatID, "Добавь пункты (каждый на новой строке)")
		return
	}

	c, err := b.checklistService.Create(user.ID, title, items)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Чек-лист создан: <b>%s</b>\n\n%s", c.Title, b.checklistService.FormatChecklist(c))
	kb := checklistKeyboard(c)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdDelChecklist deletes a checklist
func (b *Bot) cmdDelChecklist(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID или название: /delchecklist 1 или /delchecklist Тим")
		return
	}

	// Try parsing as ID first
	checklistID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		// Try finding by title
		c, err := b.checklistService.GetByTitle(user.ID, args)
		if err != nil || c == nil {
			b.SendMessage(chatID, "❌ Чек-лист не найден: "+args)
			return
		}
		checklistID = c.ID
	}

	if err := b.checklistService.Delete(checklistID, user.ID); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := "✅ Чек-лист удалён"
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Все чек-листы", "menu:checklists"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdSeedChecklists creates default checklists
func (b *Bot) cmdSeedChecklists(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	// Default checklist from TODO.md
	checklists := []struct {
		title string
		items []string
	}{
		{
			title: "Тим",
			items: []string{
				"Выспался ли он?",
				"Поел ли нормально?",
				"Какое настроение по словам Насти?",
				"Что говорит психолог?",
			},
		},
	}

	created := 0
	for _, cl := range checklists {
		_, err := b.checklistService.Create(user.ID, cl.title, cl.items)
		if err == nil {
			created++
		}
	}

	text := fmt.Sprintf("✅ Создано чек-листов: %d\n\n/checklists — посмотреть", created)
	b.SendMessage(chatID, text)
}

// cmdHistory shows completed tasks
func (b *Bot) cmdHistory(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	tasks, err := b.storage.ListCompletedTasks(user.ID, 20)
	if err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

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
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdStats shows task statistics
func (b *Bot) cmdStats(chatID int64, user *domain.User) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	monthAgo := now.AddDate(0, -1, 0)

	weekCompleted, weekCreated, _ := b.storage.GetTaskStats(user.ID, weekAgo)
	monthCompleted, monthCreated, _ := b.storage.GetTaskStats(user.ID, monthAgo)
	pendingCount, _ := b.storage.GetPendingTaskCount(user.ID)

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
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdLinkPerson links a Person from /people to a Telegram user
func (b *Bot) cmdLinkPerson(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Связать человека с Telegram:</b>

/linkperson Имя @telegram_user

<b>Примеры:</b>
/linkperson Ира @ira_username

💡 После связывания @ира в задачах будет назначать задачи этому Telegram-пользователю`
		b.SendMessage(chatID, text)
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		b.SendMessage(chatID, "Укажи: /linkperson Имя @telegram_user")
		return
	}

	personName := parts[0]
	telegramRef := parts[1]

	// Находим Person
	person, err := b.personService.GetByName(user.ID, personName)
	if err != nil || person == nil {
		b.SendMessage(chatID, "❌ Человек не найден: "+personName+"\n\n💡 Добавь через /addperson")
		return
	}

	var telegramID int64
	var displayName string

	// Пробуем распарсить как числовой ID
	if id, err := strconv.ParseInt(telegramRef, 10, 64); err == nil {
		telegramID = id
		displayName = telegramRef
	} else if strings.HasPrefix(telegramRef, "@") {
		// Ищем по @username
		username := strings.TrimPrefix(telegramRef, "@")
		telegramUser, _ := b.storage.GetUserByName(username)
		if telegramUser == nil {
			b.SendMessage(chatID, "❌ Telegram-пользователь не найден: "+telegramRef+"\n\n💡 Пользователь должен написать /start боту, или используй числовой ID")
			return
		}
		telegramID = telegramUser.TelegramID
		displayName = "@" + telegramUser.Name
	} else {
		b.SendMessage(chatID, "❌ Укажи @username или числовой Telegram ID")
		return
	}

	// Связываем
	if err := b.personService.LinkToTelegram(person.ID, telegramID); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ <b>%s</b> связан с Telegram %s\n\nТеперь @%s в задачах будет назначать задачи этому пользователю",
		person.Name, displayName, strings.ToLower(person.Name))
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Люди", "menu:people"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdShareWeekly makes a weekly event shared with family
func (b *Bot) cmdShareWeekly(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		text := `<b>Сделать событие общим:</b>

/shareweekly ID — сделать событие видимым для семьи
/unshareweekly ID — убрать из общих

<b>Пример:</b>
/shareweekly 5`
		b.SendMessage(chatID, text)
		return
	}

	eventID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID события")
		return
	}

	if err := b.scheduleService.SetShared(eventID, user.ID, true); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Событие <b>#%d</b> теперь видно всей семье 👨‍👩‍👧‍👦", eventID)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}

// cmdUnshareWeekly removes shared flag from weekly event
func (b *Bot) cmdUnshareWeekly(chatID int64, user *domain.User, args string) {
	if user == nil {
		b.SendMessage(chatID, "Сначала /start")
		return
	}

	if args == "" {
		b.SendMessage(chatID, "Укажи ID события: /unshareweekly 5")
		return
	}

	eventID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		b.SendMessage(chatID, "Неверный ID события")
		return
	}

	if err := b.scheduleService.SetShared(eventID, user.ID, false); err != nil {
		b.SendMessage(chatID, "❌ Ошибка: "+err.Error())
		return
	}

	text := fmt.Sprintf("✅ Событие <b>#%d</b> больше не общее", eventID)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Расписание", "menu:week"),
		),
	)
	b.SendMessageWithKeyboard(chatID, text, kb)
}
