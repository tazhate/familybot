package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"
)

// API Response types
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type TaskResponse struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Priority   string  `json:"priority"`
	DueDate    *string `json:"due_date,omitempty"`
	IsDone     bool    `json:"is_done"`
	PersonID   *int64  `json:"person_id,omitempty"`
	PersonName *string `json:"person_name,omitempty"`
	IsShared   bool    `json:"is_shared"`
	IsRepeat   bool    `json:"is_repeat"`
	CreatedAt  string  `json:"created_at"`
}

type PersonResponse struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	Birthday *string `json:"birthday,omitempty"`
	Age      *int    `json:"age,omitempty"`
}

type ReminderResponse struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Schedule string  `json:"schedule"`
	NextRun  *string `json:"next_run,omitempty"`
	IsActive bool    `json:"is_active"`
}

// SetupAPI registers API routes with Basic Auth
func (b *Bot) SetupAPI() {
	if b.cfg.APIUsername == "" || b.cfg.APIPassword == "" {
		return // API disabled if no credentials
	}

	// Tasks
	http.HandleFunc("/api/tasks", b.basicAuth(b.apiTasks))
	http.HandleFunc("/api/tasks/today", b.basicAuth(b.apiTasksToday))
	http.HandleFunc("/api/tasks/shared", b.basicAuth(b.apiTasksShared))
	http.HandleFunc("/api/task/", b.basicAuth(b.apiTask))

	// People
	http.HandleFunc("/api/people", b.basicAuth(b.apiPeople))
	http.HandleFunc("/api/birthdays", b.basicAuth(b.apiBirthdays))

	// Reminders
	http.HandleFunc("/api/reminders", b.basicAuth(b.apiReminders))

	// Week schedule
	http.HandleFunc("/api/week", b.basicAuth(b.apiWeek))
}

// basicAuth middleware
func (b *Bot) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != b.cfg.APIUsername || password != b.cfg.APIPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="FamilyBot API"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (b *Bot) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

func (b *Bot) jsonError(w http.ResponseWriter, err string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{Success: false, Error: err})
}

// GET /api/tasks - list tasks
// POST /api/tasks - create task
func (b *Bot) apiTasks(w http.ResponseWriter, r *http.Request) {
	userID := b.cfg.OwnerTelegramID
	chatID := userID // For API, use owner's personal chat

	switch r.Method {
	case http.MethodGet:
		tasks, err := b.taskService.ListByChat(chatID, false)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		personNames, _ := b.personService.GetNamesMap(userID)
		b.jsonResponse(w, b.tasksToResponse(tasks, personNames))

	case http.MethodPost:
		var req struct {
			Title    string `json:"title"`
			Priority string `json:"priority"`
			DueDate  string `json:"due_date"`
			PersonID *int64 `json:"person_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title == "" {
			b.jsonError(w, "Title is required", http.StatusBadRequest)
			return
		}

		priority := domain.Priority(req.Priority)
		if priority == "" {
			priority = domain.PrioritySomeday
		}

		var dueDate *time.Time
		if req.DueDate != "" {
			t, err := time.Parse("2006-01-02", req.DueDate)
			if err != nil {
				b.jsonError(w, "Invalid date format (use YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
			dueDate = &t
		}

		task, err := b.taskService.CreateFull(userID, chatID, req.Title, priority, req.PersonID, dueDate)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		personNames, _ := b.personService.GetNamesMap(userID)
		b.jsonResponse(w, b.taskToResponse(task, personNames))

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/tasks/today - tasks for today
func (b *Bot) apiTasksToday(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID
	chatID := userID

	tasks, err := b.taskService.ListForTodayByChat(chatID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	personNames, _ := b.personService.GetNamesMap(userID)
	b.jsonResponse(w, b.tasksToResponse(tasks, personNames))
}

// GET /api/tasks/shared - shared tasks
func (b *Bot) apiTasksShared(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID

	tasks, err := b.taskService.ListShared(false)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	personNames, _ := b.personService.GetNamesMap(userID)
	b.jsonResponse(w, b.tasksToResponse(tasks, personNames))
}

// GET /api/task/{id} - get task
// PUT /api/task/{id} - update task
// DELETE /api/task/{id} - delete task
// POST /api/task/{id}/done - mark done
func (b *Bot) apiTask(w http.ResponseWriter, r *http.Request) {
	userID := b.cfg.OwnerTelegramID
	chatID := userID

	// Parse task ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/task/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		b.jsonError(w, "Task ID required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.jsonError(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	// Handle sub-paths
	if len(parts) > 1 {
		switch parts[1] {
		case "done":
			if r.Method != http.MethodPost {
				b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := b.taskService.MarkDone(taskID, userID, chatID); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			b.jsonResponse(w, map[string]bool{"done": true})
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		task, err := b.taskService.Get(taskID)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if task == nil {
			b.jsonError(w, "Task not found", http.StatusNotFound)
			return
		}
		personNames, _ := b.personService.GetNamesMap(userID)
		b.jsonResponse(w, b.taskToResponse(task, personNames))

	case http.MethodPut:
		var req struct {
			Title    *string `json:"title"`
			Priority *string `json:"priority"`
			DueDate  *string `json:"due_date"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title != nil {
			if err := b.taskService.UpdateTitle(taskID, userID, chatID, *req.Title); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.Priority != nil {
			if err := b.taskService.UpdatePriority(taskID, userID, chatID, domain.Priority(*req.Priority)); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.DueDate != nil {
			var dueDate *time.Time
			if *req.DueDate != "" {
				t, err := time.Parse("2006-01-02", *req.DueDate)
				if err != nil {
					b.jsonError(w, "Invalid date format", http.StatusBadRequest)
					return
				}
				dueDate = &t
			}
			if err := b.taskService.UpdateDueDate(taskID, userID, chatID, dueDate); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		task, _ := b.taskService.Get(taskID)
		personNames, _ := b.personService.GetNamesMap(userID)
		b.jsonResponse(w, b.taskToResponse(task, personNames))

	case http.MethodDelete:
		if err := b.taskService.Delete(taskID, userID, chatID); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b.jsonResponse(w, map[string]bool{"deleted": true})

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/people - list people
func (b *Bot) apiPeople(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID

	persons, err := b.personService.List(userID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.personsToResponse(persons))
}

// GET /api/birthdays - upcoming birthdays
func (b *Bot) apiBirthdays(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID

	persons, err := b.personService.ListUpcomingBirthdays(userID, 60)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.personsToResponse(persons))
}

// GET /api/reminders - list reminders
func (b *Bot) apiReminders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID

	reminders, err := b.reminderService.List(userID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.remindersToResponse(reminders))
}

// GET /api/week - week schedule
func (b *Bot) apiWeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := b.cfg.OwnerTelegramID

	events, err := b.scheduleService.List(userID, true)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type EventResponse struct {
		ID             int64   `json:"id"`
		DayOfWeek      int     `json:"day_of_week"`
		DayName        string  `json:"day_name"`
		TimeStart      string  `json:"time_start"`
		TimeEnd        *string `json:"time_end,omitempty"`
		Title          string  `json:"title"`
		ReminderBefore int     `json:"reminder_before"`
		IsFloating     bool    `json:"is_floating"`
	}

	var result []EventResponse
	for _, e := range events {
		er := EventResponse{
			ID:             e.ID,
			DayOfWeek:      int(e.DayOfWeek),
			DayName:        e.DayName(),
			TimeStart:      e.TimeStart,
			Title:          e.Title,
			ReminderBefore: e.ReminderBefore,
			IsFloating:     e.IsFloating,
		}
		if e.TimeEnd != "" {
			er.TimeEnd = &e.TimeEnd
		}
		result = append(result, er)
	}

	b.jsonResponse(w, result)
}

// Helper: convert tasks to API response
func (b *Bot) tasksToResponse(tasks []*domain.Task, personNames map[int64]string) []TaskResponse {
	result := make([]TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, b.taskToResponse(t, personNames))
	}
	return result
}

func (b *Bot) taskToResponse(t *domain.Task, personNames map[int64]string) TaskResponse {
	tr := TaskResponse{
		ID:        t.ID,
		Title:     t.Title,
		Priority:  string(t.Priority),
		IsDone:    t.IsDone(),
		PersonID:  t.PersonID,
		IsShared:  t.IsShared,
		IsRepeat:  t.IsRepeating(),
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
	}
	if t.DueDate != nil {
		d := t.DueDate.Format("2006-01-02")
		tr.DueDate = &d
	}
	if t.PersonID != nil && personNames != nil {
		if name, ok := personNames[*t.PersonID]; ok {
			tr.PersonName = &name
		}
	}
	return tr
}

// Helper: convert persons to API response
func (b *Bot) personsToResponse(persons []*domain.Person) []PersonResponse {
	result := make([]PersonResponse, 0, len(persons))
	for _, p := range persons {
		pr := PersonResponse{
			ID:   p.ID,
			Name: p.Name,
			Role: string(p.Role),
		}
		if p.HasBirthday() {
			d := p.Birthday.Format("2006-01-02")
			pr.Birthday = &d
			if p.Birthday.Year() > 1 {
				age := p.Age()
				pr.Age = &age
			}
		}
		result = append(result, pr)
	}
	return result
}

// Helper: convert reminders to API response
func (b *Bot) remindersToResponse(reminders []*domain.Reminder) []ReminderResponse {
	result := make([]ReminderResponse, 0, len(reminders))
	for _, r := range reminders {
		rr := ReminderResponse{
			ID:       r.ID,
			Title:    r.Title,
			Type:     string(r.Type),
			Schedule: r.Schedule,
			IsActive: r.IsActive,
		}
		if r.NextRun != nil {
			d := r.NextRun.Format(time.RFC3339)
			rr.NextRun = &d
		}
		result = append(result, rr)
	}
	return result
}

// SendTelegramMessage sends a message via the bot API (for MCP to trigger notifications)
func (b *Bot) SendTelegramMessage(text string) error {
	return b.SendMessage(b.cfg.OwnerTelegramID, fmt.Sprintf("🤖 <b>API:</b>\n%s", text))
}
