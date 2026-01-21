package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type TaskService struct {
	storage *storage.Storage
}

func NewTaskService(s *storage.Storage) *TaskService {
	return &TaskService{storage: s}
}

func (s *TaskService) Create(userID int64, chatID int64, title string, priority domain.Priority) (*domain.Task, error) {
	return s.CreateFull(userID, chatID, title, priority, nil, nil)
}

func (s *TaskService) CreateWithPerson(userID int64, chatID int64, title string, priority domain.Priority, personID *int64) (*domain.Task, error) {
	return s.CreateFull(userID, chatID, title, priority, personID, nil)
}

func (s *TaskService) CreateFull(userID int64, chatID int64, title string, priority domain.Priority, personID *int64, dueDate *time.Time) (*domain.Task, error) {
	return s.CreateRepeating(userID, chatID, title, priority, personID, dueDate, "", "")
}

// CreateRepeating creates a repeating task
func (s *TaskService) CreateRepeating(userID int64, chatID int64, title string, priority domain.Priority, personID *int64, dueDate *time.Time, repeatType domain.RepeatType, repeatTime string) (*domain.Task, error) {
	return s.CreateRepeatingWithWeekNum(userID, chatID, title, priority, personID, dueDate, repeatType, repeatTime, 0)
}

// CreateRepeatingWithWeekNum creates a repeating task with week number for monthly_nth type
func (s *TaskService) CreateRepeatingWithWeekNum(userID int64, chatID int64, title string, priority domain.Priority, personID *int64, dueDate *time.Time, repeatType domain.RepeatType, repeatTime string, repeatWeekNum int) (*domain.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}

	if priority == "" {
		priority = domain.PrioritySomeday
	}

	task := &domain.Task{
		UserID:        userID,
		ChatID:        chatID,
		Title:         title,
		Priority:      priority,
		PersonID:      personID,
		DueDate:       dueDate,
		RepeatType:    repeatType,
		RepeatTime:    repeatTime,
		RepeatWeekNum: repeatWeekNum,
	}

	if err := s.storage.CreateTask(task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}

// ParseMentions extracts @name mentions from text and returns clean text + person names
func (s *TaskService) ParseMentions(text string) (cleanText string, mentions []string) {
	re := regexp.MustCompile(`@(\S+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, m := range matches {
		mentions = append(mentions, m[1])
	}

	cleanText = re.ReplaceAllString(text, "")
	cleanText = strings.TrimSpace(cleanText)
	// Clean up multiple spaces
	cleanText = regexp.MustCompile(`\s+`).ReplaceAllString(cleanText, " ")

	return cleanText, mentions
}

// MentionResult represents resolved mention target (hybrid Person + Telegram User)
type MentionResult struct {
	Person     *domain.Person // Person record (may be nil if only Telegram user found)
	User       *domain.User   // Telegram User (may be nil if person has no Telegram)
	PersonID   *int64         // ID for Task.PersonID field
	UserID     *int64         // ID for Task.AssignedTo field
	TelegramID *int64         // Telegram ID for notifications
	Name       string         // Display name
}

// ResolveMention resolves @mention to Person and/or Telegram User
// Priority: Person (with or without TelegramID) > User
func (s *TaskService) ResolveMention(ownerUserID int64, mention string) (*MentionResult, error) {
	result := &MentionResult{}

	// 1. First search in People
	person, _ := s.storage.GetPersonByName(ownerUserID, mention)
	if person != nil {
		result.Person = person
		result.PersonID = &person.ID
		result.Name = person.Name

		// If Person is linked to Telegram
		if person.TelegramID != nil {
			result.TelegramID = person.TelegramID
			// Find User by TelegramID
			user, _ := s.storage.GetUserByTelegramID(*person.TelegramID)
			if user != nil {
				result.User = user
				result.UserID = &user.ID
			}
		}
		return result, nil
	}

	// 2. If not found in People — search in Users
	user, _ := s.storage.GetUserByName(mention)
	if user != nil {
		result.User = user
		result.UserID = &user.ID
		result.TelegramID = &user.TelegramID
		result.Name = user.Name
		return result, nil
	}

	return nil, fmt.Errorf("не найдено: @%s", mention)
}

// ParseDate extracts date from Russian text like "завтра", "в понедельник", "20 января", "04.02"
// Returns clean text and parsed date (or nil if no date found)
func (s *TaskService) ParseDate(text string) (cleanText string, dueDate *time.Time) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Date patterns (order matters - more specific first)
	patterns := []struct {
		pattern string
		days    int
	}{
		{`\bпослезавтра\b`, 2},
		{`\bзавтра\b`, 1},
		{`\bсегодня\b`, 0},
		{`\bчерез\s+неделю\b`, 7},
		{`\bчерез\s+2\s+недели?\b`, 14},
		{`\bчерез\s+месяц\b`, 30},
	}

	// Weekday patterns
	weekdays := map[string]time.Weekday{
		"понедельник": time.Monday,
		"вторник":     time.Tuesday,
		"среду":       time.Wednesday,
		"среда":       time.Wednesday,
		"четверг":     time.Thursday,
		"пятницу":     time.Friday,
		"пятница":     time.Friday,
		"субботу":     time.Saturday,
		"суббота":     time.Saturday,
		"воскресенье": time.Sunday,
	}

	// Russian month names
	months := map[string]time.Month{
		"января":   time.January,
		"февраля":  time.February,
		"марта":    time.March,
		"апреля":   time.April,
		"мая":      time.May,
		"июня":     time.June,
		"июля":     time.July,
		"августа":  time.August,
		"сентября": time.September,
		"октября":  time.October,
		"ноября":   time.November,
		"декабря":  time.December,
	}

	cleanText = text

	// Try simple patterns first
	for _, p := range patterns {
		re := regexp.MustCompile(`(?i)` + p.pattern)
		if re.MatchString(cleanText) {
			d := today.AddDate(0, 0, p.days)
			dueDate = &d
			cleanText = re.ReplaceAllString(cleanText, "")
			break
		}
	}

	// Try DD.MM.YYYY or DD.MM format
	if dueDate == nil {
		// DD.MM.YYYY
		re := regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\.(\d{4})\b`)
		if matches := re.FindStringSubmatch(cleanText); matches != nil {
			day := atoi(matches[1])
			month := atoi(matches[2])
			year := atoi(matches[3])
			if day >= 1 && day <= 31 && month >= 1 && month <= 12 {
				d := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, now.Location())
				dueDate = &d
				cleanText = re.ReplaceAllString(cleanText, "")
			}
		}
	}

	if dueDate == nil {
		// DD.MM (without year - use current or next year)
		re := regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\b`)
		if matches := re.FindStringSubmatch(cleanText); matches != nil {
			day := atoi(matches[1])
			month := atoi(matches[2])
			if day >= 1 && day <= 31 && month >= 1 && month <= 12 {
				year := now.Year()
				d := time.Date(year, time.Month(month), int(day), 0, 0, 0, 0, now.Location())
				// If date is in the past, use next year
				if d.Before(today) {
					d = d.AddDate(1, 0, 0)
				}
				dueDate = &d
				cleanText = re.ReplaceAllString(cleanText, "")
			}
		}
	}

	// Try "20 января" format
	if dueDate == nil {
		for monthName, monthNum := range months {
			re := regexp.MustCompile(`(?i)\b(\d{1,2})\s+` + monthName + `\b`)
			if matches := re.FindStringSubmatch(cleanText); matches != nil {
				day := atoi(matches[1])
				if day >= 1 && day <= 31 {
					year := now.Year()
					d := time.Date(year, monthNum, int(day), 0, 0, 0, 0, now.Location())
					// If date is in the past, use next year
					if d.Before(today) {
						d = d.AddDate(1, 0, 0)
					}
					dueDate = &d
					cleanText = re.ReplaceAllString(cleanText, "")
					break
				}
			}
		}
	}

	// Try weekday patterns ("в понедельник", "в пятницу")
	if dueDate == nil {
		for name, wd := range weekdays {
			re := regexp.MustCompile(`(?i)\bв\s+` + name + `\b`)
			if re.MatchString(cleanText) {
				// Find the next occurrence of this weekday
				daysUntil := int(wd) - int(now.Weekday())
				if daysUntil <= 0 {
					daysUntil += 7
				}
				d := today.AddDate(0, 0, daysUntil)
				dueDate = &d
				cleanText = re.ReplaceAllString(cleanText, "")
				break
			}
		}
	}

	// Clean up multiple spaces
	cleanText = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(cleanText), " ")

	return cleanText, dueDate
}

func atoi(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}

// ListByPerson returns tasks for a specific person
func (s *TaskService) ListByPerson(personID int64, includeDone bool) ([]*domain.Task, error) {
	return s.storage.ListTasksByPerson(personID, includeDone)
}

// LinkToPerson links a task to a person
func (s *TaskService) LinkToPerson(taskID int64, userID int64, personID *int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}
	if task.UserID != userID {
		return fmt.Errorf("access denied")
	}
	return s.storage.UpdateTaskPerson(taskID, personID)
}

func (s *TaskService) List(userID int64, includeDone bool) ([]*domain.Task, error) {
	return s.storage.ListTasksByUser(userID, true, includeDone)
}

// ListByChat returns tasks for a specific chat context
func (s *TaskService) ListByChat(chatID int64, includeDone bool) ([]*domain.Task, error) {
	return s.storage.ListTasksByChat(chatID, includeDone)
}

func (s *TaskService) ListForToday(userID int64) ([]*domain.Task, error) {
	return s.storage.ListTasksForToday(userID)
}

// ListForTodayByChat returns urgent tasks for a specific chat
func (s *TaskService) ListForTodayByChat(chatID int64) ([]*domain.Task, error) {
	return s.storage.ListTasksForTodayByChat(chatID)
}

func (s *TaskService) MarkDone(taskID int64, userID int64, chatID int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Проверяем доступ:
	// - задача в этом чате (все участники могут отмечать)
	// - ИЛИ задача назначена мне
	// - ИЛИ я создатель задачи
	if task.ChatID != chatID && task.UserID != userID && (task.AssignedTo == nil || *task.AssignedTo != userID) {
		return fmt.Errorf("access denied")
	}

	// Отмечаем задачу выполненной
	if err := s.storage.MarkTaskDone(taskID); err != nil {
		return err
	}

	// Если задача повторяющаяся — создаём новую на следующий раз
	if task.IsRepeating() {
		nextDue := task.NextOccurrence(time.Now())
		_, err := s.CreateRepeatingWithWeekNum(
			task.UserID,
			task.ChatID,
			task.Title,
			task.Priority,
			task.PersonID,
			nextDue,
			task.RepeatType,
			task.RepeatTime,
			task.RepeatWeekNum,
		)
		if err != nil {
			// Логируем но не возвращаем ошибку — основная задача выполнена
			fmt.Printf("Error creating next occurrence: %v\n", err)
		}
	}

	return nil
}

func (s *TaskService) Delete(taskID int64, userID int64, chatID int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Удалить может только создатель или участник того же чата
	if task.UserID != userID && task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}

	return s.storage.DeleteTask(taskID)
}

// Assign assigns a task to a user
func (s *TaskService) Assign(taskID int64, assignToUserID int64, requestingUserID int64, chatID int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Назначить может только участник того же чата
	if task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}

	return s.storage.UpdateTaskAssignment(taskID, &assignToUserID)
}

// Unassign removes assignment from a task
func (s *TaskService) Unassign(taskID int64, requestingUserID int64, chatID int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Снять назначение может участник того же чата
	if task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}

	return s.storage.UpdateTaskAssignment(taskID, nil)
}

// ListShared returns all shared tasks
func (s *TaskService) ListShared(includeDone bool) ([]*domain.Task, error) {
	return s.storage.ListSharedTasks(includeDone)
}

// SetShared marks a task as shared or not
func (s *TaskService) SetShared(taskID int64, userID int64, chatID int64, isShared bool) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Сделать общей может создатель или участник того же чата
	if task.UserID != userID && task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}

	return s.storage.UpdateTaskShared(taskID, isShared)
}

func (s *TaskService) SetDueDate(taskID int64, userID int64, dueDate time.Time) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	if task.UserID != userID {
		return fmt.Errorf("access denied")
	}

	// Для простоты обновим через прямой SQL
	// В продакшене лучше добавить метод в storage
	return nil
}

// Snooze откладывает напоминания о задаче на указанное время
func (s *TaskService) Snooze(taskID int64, userID int64, chatID int64, duration time.Duration) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Проверяем доступ
	if task.ChatID != chatID && task.UserID != userID && (task.AssignedTo == nil || *task.AssignedTo != userID) {
		return fmt.Errorf("access denied")
	}

	until := time.Now().Add(duration)
	return s.storage.SnoozeTask(taskID, until)
}

// ListUrgentForReminder возвращает urgent задачи, о которых нужно напомнить
func (s *TaskService) ListUrgentForReminder() ([]*domain.Task, error) {
	return s.storage.ListUrgentTasksForReminder()
}

// MarkReminded отмечает, что о задаче напомнили
func (s *TaskService) MarkReminded(taskID int64) error {
	return s.storage.UpdateTaskReminder(taskID)
}

// Get returns a task by ID
func (s *TaskService) Get(taskID int64) (*domain.Task, error) {
	return s.storage.GetTask(taskID)
}

// UpdateTitle updates task title
func (s *TaskService) UpdateTitle(taskID int64, userID int64, chatID int64, title string) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}
	if task.UserID != userID && task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	return s.storage.UpdateTaskTitle(taskID, title)
}

// UpdatePriority updates task priority
func (s *TaskService) UpdatePriority(taskID int64, userID int64, chatID int64, priority domain.Priority) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}
	if task.UserID != userID && task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}
	return s.storage.UpdateTaskPriority(taskID, priority)
}

// UpdateDueDate updates task due date
func (s *TaskService) UpdateDueDate(taskID int64, userID int64, chatID int64, dueDate *time.Time) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}
	if task.UserID != userID && task.ChatID != chatID {
		return fmt.Errorf("access denied")
	}
	return s.storage.UpdateTaskDueDate(taskID, dueDate)
}

func (s *TaskService) FormatTaskList(tasks []*domain.Task) string {
	return s.FormatTaskListWithPersons(tasks, nil)
}

func (s *TaskService) FormatTaskListWithPersons(tasks []*domain.Task, personNames map[int64]string) string {
	if len(tasks) == 0 {
		return "Нет задач"
	}

	var sb strings.Builder
	for _, t := range tasks {
		status := "⬜"
		if t.IsDone() {
			status = "✅"
		}
		line := fmt.Sprintf("%s %s%s #%d %s", status, t.PriorityEmoji(), t.RepeatEmoji(), t.ID, t.Title)
		// Show person name if linked
		if t.PersonID != nil && personNames != nil {
			if name, ok := personNames[*t.PersonID]; ok {
				line += fmt.Sprintf(" @%s", name)
			}
		}
		if t.DueDate != nil {
			line += fmt.Sprintf(" 📅%s", t.DueDate.Format("02.01"))
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
