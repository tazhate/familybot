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

type CalendarEventResponse struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Location    string  `json:"location,omitempty"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time,omitempty"`
	AllDay      bool    `json:"all_day"`
	IsShared    bool    `json:"is_shared"`
}

type ScheduleEventResponse struct {
	ID             int64   `json:"id"`
	DayOfWeek      int     `json:"day_of_week"`
	DayName        string  `json:"day_name"`
	TimeStart      string  `json:"time_start"`
	TimeEnd        *string `json:"time_end,omitempty"`
	Title          string  `json:"title"`
	ReminderBefore int     `json:"reminder_before"`
	IsFloating     bool    `json:"is_floating"`
	IsShared       bool    `json:"is_shared"`
	IsTrackable    bool    `json:"is_trackable"`
	ChecklistID    *int64  `json:"checklist_id,omitempty"`
}

type ChecklistItemResponse struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

type ChecklistResponse struct {
	ID        int64                   `json:"id"`
	Title     string                  `json:"title"`
	Items     []ChecklistItemResponse `json:"items"`
	PersonID  *int64                  `json:"person_id,omitempty"`
	CreatedAt string                  `json:"created_at"`
}

// SetupAPI registers API routes with Basic Auth
func (b *Bot) SetupAPI() {
	if b.cfg.APIUsername == "" || b.cfg.APIPassword == "" {
		return // API disabled if no credentials
	}

	// Tasks (owner)
	http.HandleFunc("/api/tasks", b.basicAuth(b.apiTasks))
	http.HandleFunc("/api/tasks/today", b.basicAuth(b.apiTasksToday))
	http.HandleFunc("/api/tasks/shared", b.basicAuth(b.apiTasksShared))
	http.HandleFunc("/api/tasks/history", b.basicAuth(b.apiTasksHistory))
	http.HandleFunc("/api/tasks/stats", b.basicAuth(b.apiTasksStats))
	http.HandleFunc("/api/task/", b.basicAuth(b.apiTask))

	// Partner tasks (partner's own + shared)
	http.HandleFunc("/api/partner/tasks", b.basicAuth(b.apiPartnerTasks))
	http.HandleFunc("/api/partner/tasks/today", b.basicAuth(b.apiPartnerTasksToday))
	http.HandleFunc("/api/partner/task/", b.basicAuth(b.apiPartnerTask))

	// People
	http.HandleFunc("/api/people", b.basicAuth(b.apiPeople))
	http.HandleFunc("/api/birthdays", b.basicAuth(b.apiBirthdays))

	// Reminders
	http.HandleFunc("/api/reminders", b.basicAuth(b.apiReminders))
	http.HandleFunc("/api/reminder/", b.basicAuth(b.apiReminder))

	// Checklists
	http.HandleFunc("/api/checklists", b.basicAuth(b.apiChecklists))
	http.HandleFunc("/api/checklist/", b.basicAuth(b.apiChecklist))

	// Week schedule
	http.HandleFunc("/api/week", b.basicAuth(b.apiWeek))
	http.HandleFunc("/api/schedule", b.basicAuth(b.apiSchedule))
	http.HandleFunc("/api/schedule/", b.basicAuth(b.apiScheduleItem))

	// Calendar (Apple Calendar integration)
	http.HandleFunc("/api/calendar/today", b.basicAuth(b.apiCalendarToday))
	http.HandleFunc("/api/calendar/week", b.basicAuth(b.apiCalendarWeek))
	http.HandleFunc("/api/calendar/events", b.basicAuth(b.apiCalendarEvents))
	http.HandleFunc("/api/calendar/event/", b.basicAuth(b.apiCalendarEventDelete))
	http.HandleFunc("/api/calendar/sync", b.basicAuth(b.apiCalendarSync))
	http.HandleFunc("/api/calendar/list", b.basicAuth(b.apiCalendarList))

	// Todoist integration
	http.HandleFunc("/api/todoist/sync", b.basicAuth(b.apiTodoistSync))
	http.HandleFunc("/api/todoist/projects", b.basicAuth(b.apiTodoistProjects))
	http.HandleFunc("/api/todoist/sections", b.basicAuth(b.apiTodoistSections))
	http.HandleFunc("/api/todoist/reset-owner-ids", b.basicAuth(b.apiTodoistResetOwnerIDs))
	http.HandleFunc("/api/todoist/cleanup-wrong-tasks", b.basicAuth(b.apiTodoistCleanupWrongTasks))

	// Debug/Admin endpoints
	http.HandleFunc("/api/users", b.basicAuth(b.apiUsers))
	http.HandleFunc("/api/debug/tasks", b.basicAuth(b.apiDebugTasks))
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
	// Look up user by Telegram ID to get internal ID
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID // Fallback
	}
	chatID := b.cfg.OwnerTelegramID // Chat ID is always Telegram ID

	switch r.Method {
	case http.MethodGet:
		tasks, err := b.taskService.List(userID, false)
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

		// Синхронизация с Apple Calendar если есть дата
		if task.DueDate != nil && b.calendarService != nil {
			_ = b.calendarService.SyncTaskToCalendar(task)
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

		case "share":
			if r.Method != http.MethodPost {
				b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			task, err := b.storage.GetTask(taskID)
			if err != nil || task == nil {
				b.jsonError(w, "Task not found", http.StatusNotFound)
				return
			}
			task.IsShared = true
			if err := b.storage.UpdateTask(task); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			b.jsonResponse(w, map[string]bool{"shared": true})
			return

		case "unshare":
			if r.Method != http.MethodPost {
				b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			task, err := b.storage.GetTask(taskID)
			if err != nil || task == nil {
				b.jsonError(w, "Task not found", http.StatusNotFound)
				return
			}
			task.IsShared = false
			if err := b.storage.UpdateTask(task); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			b.jsonResponse(w, map[string]bool{"shared": false})
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

// ============== Partner API endpoints ==============

// ensurePartnerUser ensures the partner user exists in the database
// Creates them if they don't exist (similar to autoRegisterUser but for API)
func (b *Bot) ensurePartnerUser() (*domain.User, error) {
	if b.cfg.PartnerTelegramID == 0 {
		return nil, fmt.Errorf("partner not configured")
	}

	user, err := b.storage.GetUserByTelegramID(b.cfg.PartnerTelegramID)
	if err != nil {
		return nil, err
	}

	if user != nil {
		return user, nil
	}

	// Create partner user
	newUser := &domain.User{
		TelegramID: b.cfg.PartnerTelegramID,
		Name:       "Partner", // Default name, will be updated on first Telegram interaction
		Role:       domain.RolePartner,
	}

	if err := b.storage.CreateUser(newUser); err != nil {
		return nil, fmt.Errorf("create partner user: %w", err)
	}

	return newUser, nil
}

// GET /api/partner/tasks - list partner's own tasks + shared tasks
// POST /api/partner/tasks - create task as partner
func (b *Bot) apiPartnerTasks(w http.ResponseWriter, r *http.Request) {
	partnerUser, err := b.ensurePartnerUser()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusForbidden)
		return
	}

	userID := partnerUser.TelegramID
	chatID := userID

	switch r.Method {
	case http.MethodGet:
		// Get partner's own tasks
		ownTasks, err := b.taskService.ListByChat(chatID, false)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get shared tasks
		sharedTasks, err := b.taskService.ListShared(false)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Merge: own tasks + shared (excluding duplicates)
		taskMap := make(map[int64]*domain.Task)
		for _, t := range ownTasks {
			taskMap[t.ID] = t
		}
		for _, t := range sharedTasks {
			if _, exists := taskMap[t.ID]; !exists {
				taskMap[t.ID] = t
			}
		}

		var tasks []*domain.Task
		for _, t := range taskMap {
			tasks = append(tasks, t)
		}

		// Sort by priority and date (urgent first)
		// Simple bubble sort is fine for small lists
		for i := 0; i < len(tasks); i++ {
			for j := i + 1; j < len(tasks); j++ {
				// Urgent < Week < Someday
				pi := priorityOrder(tasks[i].Priority)
				pj := priorityOrder(tasks[j].Priority)
				if pi > pj || (pi == pj && tasks[i].ID > tasks[j].ID) {
					tasks[i], tasks[j] = tasks[j], tasks[i]
				}
			}
		}

		personNames, _ := b.personService.GetNamesMap(b.cfg.OwnerTelegramID)
		b.jsonResponse(w, b.tasksToResponse(tasks, personNames))

	case http.MethodPost:
		var req struct {
			Title    string `json:"title"`
			Priority string `json:"priority"`
			DueDate  string `json:"due_date"`
			PersonID *int64 `json:"person_id"`
			IsShared bool   `json:"is_shared"`
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

		// If shared flag set, mark as shared
		if req.IsShared {
			_ = b.taskService.SetShared(task.ID, userID, chatID, true)
			task.IsShared = true
		}

		personNames, _ := b.personService.GetNamesMap(b.cfg.OwnerTelegramID)
		b.jsonResponse(w, b.taskToResponse(task, personNames))

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// priorityOrder returns numeric order for priority (lower = higher priority)
func priorityOrder(p domain.Priority) int {
	switch p {
	case domain.PriorityUrgent:
		return 0
	case domain.PriorityWeek:
		return 1
	default:
		return 2
	}
}

// GET /api/partner/tasks/today - partner's tasks for today
func (b *Bot) apiPartnerTasksToday(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	partnerUser, err := b.ensurePartnerUser()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusForbidden)
		return
	}

	chatID := partnerUser.TelegramID

	// Get partner's today tasks
	ownTasks, err := b.taskService.ListForTodayByChat(chatID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get shared urgent tasks too
	sharedTasks, err := b.taskService.ListShared(false)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Merge: own today tasks + shared urgent
	taskMap := make(map[int64]*domain.Task)
	for _, t := range ownTasks {
		taskMap[t.ID] = t
	}
	for _, t := range sharedTasks {
		// Include shared tasks that are urgent or due today
		if t.Priority == domain.PriorityUrgent || (t.DueDate != nil && isToday(*t.DueDate)) {
			if _, exists := taskMap[t.ID]; !exists {
				taskMap[t.ID] = t
			}
		}
	}

	var tasks []*domain.Task
	for _, t := range taskMap {
		tasks = append(tasks, t)
	}

	personNames, _ := b.personService.GetNamesMap(b.cfg.OwnerTelegramID)
	b.jsonResponse(w, b.tasksToResponse(tasks, personNames))
}

// isToday checks if date is today
func isToday(t time.Time) bool {
	now := time.Now()
	return t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day()
}

// GET /api/partner/task/{id} - get task (own or shared)
// PUT /api/partner/task/{id} - update task (own or shared)
// DELETE /api/partner/task/{id} - delete task (own only!)
// POST /api/partner/task/{id}/done - mark done (own or shared)
func (b *Bot) apiPartnerTask(w http.ResponseWriter, r *http.Request) {
	partnerUser, err := b.ensurePartnerUser()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusForbidden)
		return
	}

	userID := partnerUser.TelegramID
	chatID := userID

	// Parse task ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/partner/task/")
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

	// Get task first to check access
	task, err := b.taskService.Get(taskID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if task == nil {
		b.jsonError(w, "Task not found", http.StatusNotFound)
		return
	}

	// Check access: partner can access own tasks OR shared tasks
	isOwnTask := task.UserID == userID || task.ChatID == chatID
	isSharedTask := task.IsShared

	if !isOwnTask && !isSharedTask {
		b.jsonError(w, "Access denied", http.StatusForbidden)
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
			// Can mark done own or shared tasks
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
		personNames, _ := b.personService.GetNamesMap(b.cfg.OwnerTelegramID)
		b.jsonResponse(w, b.taskToResponse(task, personNames))

	case http.MethodPut:
		// Can update own tasks and shared tasks
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
				// Try with owner context for shared tasks
				if isSharedTask && !isOwnTask {
					_ = b.taskService.UpdateTitle(taskID, task.UserID, task.ChatID, *req.Title)
				} else {
					b.jsonError(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		if req.Priority != nil {
			if err := b.taskService.UpdatePriority(taskID, userID, chatID, domain.Priority(*req.Priority)); err != nil {
				if isSharedTask && !isOwnTask {
					_ = b.taskService.UpdatePriority(taskID, task.UserID, task.ChatID, domain.Priority(*req.Priority))
				} else {
					b.jsonError(w, err.Error(), http.StatusInternalServerError)
					return
				}
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
				if isSharedTask && !isOwnTask {
					_ = b.taskService.UpdateDueDate(taskID, task.UserID, task.ChatID, dueDate)
				} else {
					b.jsonError(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		updatedTask, _ := b.taskService.Get(taskID)
		personNames, _ := b.personService.GetNamesMap(b.cfg.OwnerTelegramID)
		b.jsonResponse(w, b.taskToResponse(updatedTask, personNames))

	case http.MethodDelete:
		// Can only delete OWN tasks, not owner's shared tasks
		if !isOwnTask {
			b.jsonError(w, "Cannot delete: not your task", http.StatusForbidden)
			return
		}
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
// GET /api/people - list people
// POST /api/people - create person
func (b *Bot) apiPeople(w http.ResponseWriter, r *http.Request) {
	userID := b.cfg.OwnerTelegramID

	switch r.Method {
	case http.MethodGet:
		persons, err := b.personService.List(userID)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b.jsonResponse(w, b.personsToResponse(persons))

	case http.MethodPost:
		var req struct {
			Name     string  `json:"name"`
			Role     string  `json:"role"`
			Birthday *string `json:"birthday"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			b.jsonError(w, "Name is required", http.StatusBadRequest)
			return
		}

		role := domain.PersonRole(req.Role)
		if role == "" {
			role = domain.RoleContact // default
		}

		var birthday *time.Time
		if req.Birthday != nil && *req.Birthday != "" {
			t, err := time.Parse("2006-01-02", *req.Birthday)
			if err != nil {
				b.jsonError(w, "Invalid birthday format (use YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
			birthday = &t
		}

		person := &domain.Person{
			UserID:   userID,
			Name:     req.Name,
			Role:     role,
			Birthday: birthday,
		}

		if err := b.storage.CreatePerson(person); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		persons := []*domain.Person{person}
		b.jsonResponse(w, b.personsToResponse(persons)[0])

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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

// ============== Schedule API endpoints ==============

// GET /api/schedule - list schedule events
// POST /api/schedule - create schedule event
func (b *Bot) apiSchedule(w http.ResponseWriter, r *http.Request) {
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		events, err := b.scheduleService.List(user.ID, true)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b.jsonResponse(w, b.scheduleEventsToResponse(events))

	case http.MethodPost:
		var req struct {
			Day            string `json:"day"`       // Пн, Вт, etc.
			TimeStart      string `json:"time"`      // 10:00 or 10:00-12:00
			Title          string `json:"title"`
			ReminderBefore *int   `json:"reminder"`  // minutes before (optional)
			IsTrackable    *bool  `json:"trackable"` // is trackable (optional)
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title == "" {
			b.jsonError(w, "Title is required", http.StatusBadRequest)
			return
		}
		if req.Day == "" {
			b.jsonError(w, "Day is required (Пн, Вт, Ср...)", http.StatusBadRequest)
			return
		}
		if req.TimeStart == "" {
			b.jsonError(w, "Time is required (10:00 or 10:00-12:00)", http.StatusBadRequest)
			return
		}

		day, ok := domain.ParseWeekday(strings.ToLower(req.Day))
		if !ok {
			b.jsonError(w, "Invalid day (use Пн, Вт, Ср, Чт, Пт, Сб, Вс)", http.StatusBadRequest)
			return
		}

		// Parse time (could be "17:30" or "16:00-20:00")
		timeStart := req.TimeStart
		timeEnd := ""
		if strings.Contains(req.TimeStart, "-") {
			parts := strings.Split(req.TimeStart, "-")
			timeStart = parts[0]
			if len(parts) > 1 {
				timeEnd = parts[1]
			}
		}

		reminderBefore := 0
		if req.ReminderBefore != nil {
			reminderBefore = *req.ReminderBefore
		}

		event, err := b.scheduleService.Create(user.ID, day, timeStart, timeEnd, req.Title, reminderBefore)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set trackable if specified
		if req.IsTrackable != nil && *req.IsTrackable {
			_ = b.scheduleService.SetTrackable(event.ID, user.ID, true)
			event.IsTrackable = true
		}

		// Sync to Apple Calendar
		if b.calendarService != nil {
			_ = b.calendarService.SyncWeeklyEventToCalendar(event.ID, int(event.DayOfWeek), event.TimeStart, event.TimeEnd, event.Title, event.IsFloating, nil)
		}

		b.jsonResponse(w, b.scheduleEventToResponse(event))

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/schedule/{id} - get schedule event
// PUT /api/schedule/{id} - update schedule event
// DELETE /api/schedule/{id} - delete schedule event
func (b *Bot) apiScheduleItem(w http.ResponseWriter, r *http.Request) {
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	// Parse event ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/schedule/")
	if path == "" {
		b.jsonError(w, "Event ID required", http.StatusBadRequest)
		return
	}

	eventID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		b.jsonError(w, "Invalid event ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		event, err := b.scheduleService.Get(eventID)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if event == nil {
			b.jsonError(w, "Event not found", http.StatusNotFound)
			return
		}
		b.jsonResponse(w, b.scheduleEventToResponse(event))

	case http.MethodPut:
		var req struct {
			Title       *string `json:"title"`
			Day         *string `json:"day"`
			Time        *string `json:"time"`
			IsTrackable *bool   `json:"trackable"`
			IsShared    *bool   `json:"shared"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title != nil {
			if err := b.scheduleService.UpdateTitle(eventID, user.ID, *req.Title); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.Day != nil {
			day, ok := domain.ParseWeekday(strings.ToLower(*req.Day))
			if !ok {
				b.jsonError(w, "Invalid day", http.StatusBadRequest)
				return
			}
			if err := b.scheduleService.UpdateDay(eventID, user.ID, day); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.Time != nil {
			timeStart := *req.Time
			timeEnd := ""
			if strings.Contains(*req.Time, "-") {
				parts := strings.Split(*req.Time, "-")
				timeStart = parts[0]
				if len(parts) > 1 {
					timeEnd = parts[1]
				}
			}
			if err := b.scheduleService.UpdateTime(eventID, user.ID, timeStart, timeEnd); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.IsTrackable != nil {
			if err := b.scheduleService.SetTrackable(eventID, user.ID, *req.IsTrackable); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if req.IsShared != nil {
			if err := b.scheduleService.SetShared(eventID, user.ID, *req.IsShared); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// Sync to Apple Calendar if any changes were made
		event, _ := b.scheduleService.Get(eventID)
		if b.calendarService != nil && event != nil && (req.Title != nil || req.Day != nil || req.Time != nil) {
			var floatingDays []int
			if event.IsFloating {
				for _, d := range event.GetFloatingDays() {
					floatingDays = append(floatingDays, int(d))
				}
			}
			_ = b.calendarService.SyncWeeklyEventToCalendar(event.ID, int(event.DayOfWeek), event.TimeStart, event.TimeEnd, event.Title, event.IsFloating, floatingDays)
		}

		b.jsonResponse(w, b.scheduleEventToResponse(event))

	case http.MethodDelete:
		if err := b.scheduleService.Delete(eventID, user.ID); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Delete from Apple Calendar
		if b.calendarService != nil {
			_ = b.calendarService.DeleteWeeklyEventFromCalendar(eventID)
		}

		b.jsonResponse(w, map[string]bool{"deleted": true})

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Helper: convert schedule events to API response
func (b *Bot) scheduleEventsToResponse(events []*domain.WeeklyEvent) []ScheduleEventResponse {
	result := make([]ScheduleEventResponse, 0, len(events))
	for _, e := range events {
		result = append(result, b.scheduleEventToResponse(e))
	}
	return result
}

func (b *Bot) scheduleEventToResponse(e *domain.WeeklyEvent) ScheduleEventResponse {
	resp := ScheduleEventResponse{
		ID:             e.ID,
		DayOfWeek:      int(e.DayOfWeek),
		DayName:        e.DayName(),
		TimeStart:      e.TimeStart,
		Title:          e.Title,
		ReminderBefore: e.ReminderBefore,
		IsFloating:     e.IsFloating,
		IsShared:       e.IsShared,
		IsTrackable:    e.IsTrackable,
	}
	if e.TimeEnd != "" {
		resp.TimeEnd = &e.TimeEnd
	}
	return resp
}

// ============== Calendar API endpoints ==============

// GET /api/calendar/today - calendar events for today
func (b *Bot) apiCalendarToday(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	userID := b.cfg.OwnerTelegramID
	user, _ := b.storage.GetUserByTelegramID(userID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	events, err := b.calendarService.ListToday(user.ID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.calendarEventsToResponse(events))
}

// GET /api/calendar/week - calendar events for the week
func (b *Bot) apiCalendarWeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	userID := b.cfg.OwnerTelegramID
	user, _ := b.storage.GetUserByTelegramID(userID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	events, err := b.calendarService.ListWeek(user.ID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.calendarEventsToResponse(events))
}

// POST /api/calendar/events - create calendar event
func (b *Bot) apiCalendarEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Location    string `json:"location"`
		StartTime   string `json:"start_time"` // YYYY-MM-DD HH:MM or YYYY-MM-DD
		EndTime     string `json:"end_time"`   // YYYY-MM-DD HH:MM or YYYY-MM-DD (optional)
		AllDay      bool   `json:"all_day"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		b.jsonError(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.StartTime == "" {
		b.jsonError(w, "Start time is required", http.StatusBadRequest)
		return
	}

	// Parse start time
	var startTime time.Time
	var err error
	if len(req.StartTime) == 10 {
		// Date only: YYYY-MM-DD
		startTime, err = time.Parse("2006-01-02", req.StartTime)
		req.AllDay = true
	} else {
		// Date and time: YYYY-MM-DD HH:MM
		startTime, err = time.Parse("2006-01-02 15:04", req.StartTime)
	}
	if err != nil {
		b.jsonError(w, "Invalid start_time format (use YYYY-MM-DD or YYYY-MM-DD HH:MM)", http.StatusBadRequest)
		return
	}

	// Parse end time (optional)
	var endTime time.Time
	if req.EndTime != "" {
		if len(req.EndTime) == 10 {
			endTime, err = time.Parse("2006-01-02", req.EndTime)
		} else {
			endTime, err = time.Parse("2006-01-02 15:04", req.EndTime)
		}
		if err != nil {
			b.jsonError(w, "Invalid end_time format", http.StatusBadRequest)
			return
		}
	} else if !req.AllDay {
		// Default: 1 hour duration
		endTime = startTime.Add(time.Hour)
	} else {
		endTime = startTime
	}

	userID := b.cfg.OwnerTelegramID
	user, _ := b.storage.GetUserByTelegramID(userID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	event, err := b.calendarService.CreateEvent(user.ID, req.Title, startTime, endTime, req.Location, req.AllDay)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, b.calendarEventToResponse(event))
}

// DELETE /api/calendar/event/{id} - delete calendar event
func (b *Bot) apiCalendarEventDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse event ID from path: /api/calendar/event/123
	path := r.URL.Path
	idStr := strings.TrimPrefix(path, "/api/calendar/event/")
	eventID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		b.jsonError(w, "Invalid event ID", http.StatusBadRequest)
		return
	}

	userID := b.cfg.OwnerTelegramID
	user, _ := b.storage.GetUserByTelegramID(userID)
	if user == nil {
		b.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	if err := b.calendarService.DeleteEvent(eventID, user.ID); err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, map[string]interface{}{
		"deleted": eventID,
		"message": "Event deleted",
	})
}

// GET /api/calendar/list - list available Apple calendars
func (b *Bot) apiCalendarList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	calendars, err := b.calendarService.DiscoverCalendars()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type CalendarItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	var result []CalendarItem
	for _, c := range calendars {
		result = append(result, CalendarItem{
			ID:   c.ID,
			Name: c.DisplayName,
			URL:  c.URL,
		})
	}

	b.jsonResponse(w, result)
}

// POST /api/calendar/sync - sync with Apple Calendar
func (b *Bot) apiCalendarSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.calendarService == nil {
		b.jsonError(w, "Calendar not configured", http.StatusServiceUnavailable)
		return
	}

	// Sync calendar events FROM Apple
	result, err := b.calendarService.SyncFromApple()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync weekly schedule events TO Apple
	scheduleSynced := 0
	if b.scheduleService != nil {
		user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
		if user != nil {
			events, err := b.scheduleService.List(user.ID, true)
			if err == nil {
				for _, e := range events {
					var floatingDays []int
					if e.IsFloating {
						for _, d := range e.GetFloatingDays() {
							floatingDays = append(floatingDays, int(d))
						}
					}
					if err := b.calendarService.SyncWeeklyEventToCalendar(e.ID, int(e.DayOfWeek), e.TimeStart, e.TimeEnd, e.Title, e.IsFloating, floatingDays); err == nil {
						scheduleSynced++
					}
				}
			}
		}
	}

	b.jsonResponse(w, map[string]interface{}{
		"from_apple": map[string]interface{}{
			"added":   result.Added,
			"updated": result.Updated,
			"deleted": result.Deleted,
		},
		"to_apple": map[string]interface{}{
			"schedule_synced": scheduleSynced,
		},
		"errors":  result.Errors,
		"message": fmt.Sprintf("Синхронизация завершена: из Apple добавлено %d, обновлено %d, удалено %d; в Apple синхронизировано %d событий расписания", result.Added, result.Updated, result.Deleted, scheduleSynced),
	})
}

// Helper: convert calendar events to API response
func (b *Bot) calendarEventsToResponse(events []*domain.CalendarEvent) []CalendarEventResponse {
	result := make([]CalendarEventResponse, 0, len(events))
	for _, e := range events {
		result = append(result, b.calendarEventToResponse(e))
	}
	return result
}

func (b *Bot) calendarEventToResponse(e *domain.CalendarEvent) CalendarEventResponse {
	resp := CalendarEventResponse{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		Location:    e.Location,
		AllDay:      e.AllDay,
		IsShared:    e.IsShared,
	}

	if e.AllDay {
		resp.StartTime = e.StartTime.Format("2006-01-02")
		if !e.EndTime.IsZero() && !e.EndTime.Equal(e.StartTime) {
			resp.EndTime = e.EndTime.Format("2006-01-02")
		}
	} else {
		resp.StartTime = e.StartTime.Format("2006-01-02 15:04")
		if !e.EndTime.IsZero() {
			resp.EndTime = e.EndTime.Format("2006-01-02 15:04")
		}
	}

	return resp
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

// ============== Todoist API endpoints ==============

// POST /api/todoist/sync - sync with Todoist
func (b *Bot) apiTodoistSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.todoistService == nil || !b.todoistService.IsConfigured() {
		b.jsonError(w, "Todoist not configured", http.StatusServiceUnavailable)
		return
	}

	result, err := b.todoistService.Sync()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, map[string]interface{}{
		"from_todoist": map[string]interface{}{
			"added":   result.FromTodoist.Added,
			"updated": result.FromTodoist.Updated,
			"deleted": result.FromTodoist.Deleted,
		},
		"to_todoist": map[string]interface{}{
			"added":   result.ToTodoist.Added,
			"updated": result.ToTodoist.Updated,
			"deleted": result.ToTodoist.Deleted,
		},
		"errors":  result.Errors,
		"message": b.todoistService.FormatSyncResult(result),
	})
}

// GET /api/todoist/projects - list Todoist projects
func (b *Bot) apiTodoistProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.todoistService == nil || !b.todoistService.IsConfigured() {
		b.jsonError(w, "Todoist not configured", http.StatusServiceUnavailable)
		return
	}

	projects, err := b.todoistService.GetProjects()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type ProjectItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	var result []ProjectItem
	for _, p := range projects {
		result = append(result, ProjectItem{
			ID:   p.ID,
			Name: p.Name,
		})
	}

	b.jsonResponse(w, result)
}

// GET /api/todoist/sections - list Todoist sections for a project
func (b *Bot) apiTodoistSections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.todoistService == nil || !b.todoistService.IsConfigured() {
		b.jsonError(w, "Todoist not configured", http.StatusServiceUnavailable)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	sections, err := b.todoistService.GetSections(projectID)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b.jsonResponse(w, sections)
}

// POST /api/todoist/reset-owner-ids - reset todoist_id for owner's tasks to force resync
func (b *Bot) apiTodoistResetOwnerIDs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get owner user ID from config
	ownerUser, err := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	if err != nil {
		b.jsonError(w, "Owner user not found", http.StatusInternalServerError)
		return
	}

	// Get all active tasks for owner
	tasks, err := b.storage.ListTasksByUser(ownerUser.ID, true, false)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reset todoist_id for all non-repeating tasks
	count := 0
	for _, task := range tasks {
		if task.TodoistID != "" && !task.IsRepeating() {
			if err := b.storage.UpdateTaskTodoistID(task.ID, ""); err != nil {
				b.jsonError(w, fmt.Sprintf("Failed to reset task %d: %v", task.ID, err), http.StatusInternalServerError)
				return
			}
			count++
		}
	}

	b.jsonResponse(w, map[string]interface{}{
		"reset_count": count,
		"message":     fmt.Sprintf("Reset todoist_id for %d tasks", count),
	})
}

// POST /api/todoist/cleanup-wrong-tasks - delete tasks that belong to partner but assigned to owner
func (b *Bot) apiTodoistCleanupWrongTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.todoistService == nil || !b.todoistService.IsConfigured() {
		b.jsonError(w, "Todoist not configured", http.StatusServiceUnavailable)
		return
	}

	// Get owner user ID
	ownerUser, err := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	if err != nil {
		b.jsonError(w, "Owner user not found", http.StatusInternalServerError)
		return
	}

	// Get all active tasks for owner that are marked as shared
	tasks, err := b.storage.ListTasksByUser(ownerUser.ID, true, false)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete all shared tasks with todoist_id (these are likely imported from wrong section)
	count := 0
	for _, task := range tasks {
		if task.IsShared && task.TodoistID != "" {
			if err := b.storage.DeleteTask(task.ID); err != nil {
				b.jsonError(w, fmt.Sprintf("Failed to delete task %d: %v", task.ID, err), http.StatusInternalServerError)
				return
			}
			count++
		}
	}

	b.jsonResponse(w, map[string]interface{}{
		"deleted_count": count,
		"message":       fmt.Sprintf("Deleted %d wrong tasks", count),
	})
}

// GET /api/users - list all users (debug)
func (b *Bot) apiUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users, err := b.storage.ListUsers()
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type UserItem struct {
		ID         int64  `json:"id"`
		TelegramID int64  `json:"telegram_id"`
		Name       string `json:"name"`
		Role       string `json:"role"`
	}

	var result []UserItem
	for _, u := range users {
		result = append(result, UserItem{
			ID:         u.ID,
			TelegramID: u.TelegramID,
			Name:       u.Name,
			Role:       string(u.Role),
		})
	}

	// Also show configured owner ID
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":           true,
		"data":              result,
		"owner_telegram_id": b.cfg.OwnerTelegramID,
	}
	json.NewEncoder(w).Encode(response)
}

// GET /api/debug/tasks - list tasks with full info for debugging
func (b *Bot) apiDebugTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get owner user
	ownerUser, err := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	if err != nil || ownerUser == nil {
		b.jsonError(w, "Owner user not found", http.StatusInternalServerError)
		return
	}

	// Get all tasks for owner (including shared, excluding done)
	tasks, err := b.storage.ListTasksByUser(ownerUser.ID, true, false)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type DebugTask struct {
		ID          int64   `json:"id"`
		UserID      int64   `json:"user_id"`
		Title       string  `json:"title"`
		IsRepeating bool    `json:"is_repeating"`
		TodoistID   string  `json:"todoist_id"`
		IsShared    bool    `json:"is_shared"`
	}

	var result []DebugTask
	var withoutTodoist int
	var repeating int
	for _, t := range tasks {
		result = append(result, DebugTask{
			ID:          t.ID,
			UserID:      t.UserID,
			Title:       t.Title,
			IsRepeating: t.IsRepeating(),
			TodoistID:   t.TodoistID,
			IsShared:    t.IsShared,
		})
		if t.TodoistID == "" {
			withoutTodoist++
		}
		if t.IsRepeating() {
			repeating++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":           true,
		"owner_user_id":     ownerUser.ID,
		"total_tasks":       len(tasks),
		"without_todoist":   withoutTodoist,
		"repeating":         repeating,
		"can_sync_to_todoist": withoutTodoist - repeating,
		"tasks":             result,
	}
	json.NewEncoder(w).Encode(response)
}

// GET /api/checklists - list checklists
// POST /api/checklists - create checklist
func (b *Bot) apiChecklists(w http.ResponseWriter, r *http.Request) {
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID
	}

	switch r.Method {
	case http.MethodGet:
		checklists, err := b.checklistService.List(userID)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b.jsonResponse(w, b.checklistsToResponse(checklists))

	case http.MethodPost:
		var req struct {
			Title string   `json:"title"`
			Items []string `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Title == "" {
			b.jsonError(w, "Title is required", http.StatusBadRequest)
			return
		}
		if len(req.Items) == 0 {
			b.jsonError(w, "At least one item is required", http.StatusBadRequest)
			return
		}

		checklist, err := b.checklistService.Create(userID, req.Title, req.Items)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		b.jsonResponse(w, b.checklistToResponse(checklist))

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/checklist/:id - get checklist
// DELETE /api/checklist/:id - delete checklist
// PUT /api/checklist/:id/check/:index - check item
func (b *Bot) apiChecklist(w http.ResponseWriter, r *http.Request) {
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/checklist/")
	parts := strings.Split(path, "/")

	if parts[0] == "" {
		b.jsonError(w, "Checklist ID required", http.StatusBadRequest)
		return
	}

	checklistID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.jsonError(w, "Invalid checklist ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		checklist, err := b.storage.GetChecklist(checklistID)
		if err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if checklist == nil || checklist.UserID != userID {
			b.jsonError(w, "Checklist not found", http.StatusNotFound)
			return
		}
		b.jsonResponse(w, b.checklistToResponse(checklist))

	case http.MethodDelete:
		checklist, err := b.storage.GetChecklist(checklistID)
		if err != nil || checklist == nil || checklist.UserID != userID {
			b.jsonError(w, "Checklist not found", http.StatusNotFound)
			return
		}

		if err := b.storage.DeleteChecklist(checklistID); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b.jsonResponse(w, map[string]string{"message": "Checklist deleted"})

	case http.MethodPut:
		// Check if this is /check/:index operation
		if len(parts) >= 3 && parts[1] == "check" {
			index, err := strconv.Atoi(parts[2])
			if err != nil {
				b.jsonError(w, "Invalid item index", http.StatusBadRequest)
				return
			}

			checklist, err := b.storage.GetChecklist(checklistID)
			if err != nil || checklist == nil || checklist.UserID != userID {
				b.jsonError(w, "Checklist not found", http.StatusNotFound)
				return
			}

			if !checklist.CheckItem(index) {
				b.jsonError(w, "Invalid item index", http.StatusBadRequest)
				return
			}

			if err := b.storage.UpdateChecklistItems(checklistID, checklist.Items); err != nil {
				b.jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}

			b.jsonResponse(w, b.checklistToResponse(checklist))
		} else {
			b.jsonError(w, "Invalid operation", http.StatusBadRequest)
		}

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /api/reminder/ - create reminder for task
// PUT /api/reminder/:id - update reminder
// DELETE /api/reminder/:id - delete reminder
func (b *Bot) apiReminder(w http.ResponseWriter, r *http.Request) {
	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/reminder/")

	switch r.Method {
	case http.MethodPost:
		var req struct {
			TaskID   int64  `json:"task_id"`
			Type     string `json:"type"`
			Schedule string `json:"schedule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			b.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.TaskID == 0 {
			b.jsonError(w, "task_id is required", http.StatusBadRequest)
			return
		}

		// Verify task belongs to user
		task, err := b.storage.GetTask(req.TaskID)
		if err != nil || task == nil || task.UserID != userID {
			b.jsonError(w, "Task not found", http.StatusNotFound)
			return
		}

		// Create TaskReminder (for tasks)
		taskReminder := &domain.TaskReminder{
			TaskID:       req.TaskID,
			RemindBefore: 60, // Default 1 hour
		}

		if err := b.storage.CreateTaskReminder(taskReminder); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		b.jsonResponse(w, map[string]interface{}{
			"id":            taskReminder.ID,
			"task_id":       taskReminder.TaskID,
			"remind_before": taskReminder.RemindBefore,
		})

	case http.MethodPut:
		// Not currently supported - task reminders are managed via task endpoints
		b.jsonError(w, "Update not supported for task reminders", http.StatusNotImplemented)

	case http.MethodDelete:
		if path == "" {
			b.jsonError(w, "Reminder ID required", http.StatusBadRequest)
			return
		}

		reminderID, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			b.jsonError(w, "Invalid reminder ID", http.StatusBadRequest)
			return
		}

		if err := b.storage.DeleteTaskReminder(reminderID); err != nil {
			b.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		b.jsonResponse(w, map[string]string{"message": "Reminder deleted"})

	default:
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Bot) checklistToResponse(c *domain.Checklist) ChecklistResponse {
	items := make([]ChecklistItemResponse, len(c.Items))
	for i, item := range c.Items {
		items[i] = ChecklistItemResponse{
			Text:    item.Text,
			Checked: item.Checked,
		}
	}

	return ChecklistResponse{
		ID:        c.ID,
		Title:     c.Title,
		Items:     items,
		PersonID:  c.PersonID,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
}

func (b *Bot) checklistsToResponse(checklists []*domain.Checklist) []ChecklistResponse {
	result := make([]ChecklistResponse, len(checklists))
	for i, c := range checklists {
		result[i] = b.checklistToResponse(c)
	}
	return result
}

// GET /api/tasks/history - completed tasks history
func (b *Bot) apiTasksHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID
	}

	// Get completed tasks (last 50)
	tasks, err := b.taskService.List(userID, true) // includeDone = true
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter only completed tasks
	var completedTasks []*domain.Task
	for _, t := range tasks {
		if t.IsDone() {
			completedTasks = append(completedTasks, t)
			if len(completedTasks) >= 50 {
				break
			}
		}
	}

	personNames, _ := b.personService.GetNamesMap(userID)
	b.jsonResponse(w, b.tasksToResponse(completedTasks, personNames))
}

// GET /api/tasks/stats - task statistics
func (b *Bot) apiTasksStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		b.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, _ := b.storage.GetUserByTelegramID(b.cfg.OwnerTelegramID)
	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = b.cfg.OwnerTelegramID
	}

	// Get all tasks (including done)
	allTasks, err := b.taskService.List(userID, true)
	if err != nil {
		b.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate statistics
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	monthAgo := now.AddDate(0, -1, 0)

	stats := map[string]int{
		"active":        0,
		"week_done":     0,
		"week_created":  0,
		"month_done":    0,
		"month_created": 0,
	}

	for _, t := range allTasks {
		if t.IsDone() {
			// Use DoneAt for completed tasks
			if t.DoneAt != nil && t.DoneAt.After(weekAgo) {
				stats["week_done"]++
			}
			if t.DoneAt != nil && t.DoneAt.After(monthAgo) {
				stats["month_done"]++
			}
		} else {
			stats["active"]++
		}

		if t.CreatedAt.After(weekAgo) {
			stats["week_created"]++
		}
		if t.CreatedAt.After(monthAgo) {
			stats["month_created"]++
		}
	}

	b.jsonResponse(w, stats)
}
