package service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/clients/todoist"
	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

// TodoistService handles Todoist integration
type TodoistService struct {
	storage       *storage.Storage
	client        *todoist.Client
	ownerUserID   int64
	partnerUserID int64
}

// NewTodoistService creates a new Todoist service
func NewTodoistService(s *storage.Storage, client *todoist.Client, ownerUserID int64, partnerUserID int64) *TodoistService {
	return &TodoistService{
		storage:       s,
		client:        client,
		ownerUserID:   ownerUserID,
		partnerUserID: partnerUserID,
	}
}

// IsConfigured returns true if Todoist client is configured
func (s *TodoistService) IsConfigured() bool {
	return s.client != nil && s.client.IsConfigured()
}

// SyncResult contains sync operation results
type TodoistSyncResult struct {
	FromTodoist struct {
		Added   int
		Updated int
		Deleted int
	}
	ToTodoist struct {
		Added   int
		Updated int
		Deleted int
	}
	Errors []string
}

// Sync performs two-way sync between FamilyBot and Todoist
func (s *TodoistService) Sync() (*TodoistSyncResult, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("Todoist not configured")
	}

	result := &TodoistSyncResult{}

	// Sync owner's tasks (from owner's section)
	ownerSectionID := s.client.GetSectionID()
	if ownerSectionID != "" {
		s.syncUserTasks(s.ownerUserID, ownerSectionID, result)
	} else {
		// Fallback: sync all tasks for owner if no section specified
		s.syncUserTasks(s.ownerUserID, "", result)
	}

	// Sync partner's tasks (from partner's section)
	partnerSectionID := s.client.GetPartnerSectionID()
	if s.partnerUserID != 0 && partnerSectionID != "" {
		s.syncUserTasks(s.partnerUserID, partnerSectionID, result)
	}

	return result, nil
}

// syncUserTasks syncs tasks for a specific user and Todoist section
func (s *TodoistService) syncUserTasks(userID int64, sectionID string, result *TodoistSyncResult) {
	// Get tasks from Todoist (filtered by section if specified)
	var todoistTasks []todoist.Task
	var err error
	if sectionID != "" {
		todoistTasks, err = s.client.GetTasksBySection(sectionID)
	} else {
		todoistTasks, err = s.client.GetTasks("")
	}
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("get Todoist tasks for user %d: %v", userID, err))
		return
	}

	// Get local tasks for this user
	localTasks, err := s.storage.ListTasksByUser(userID, true, false) // Active tasks only
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("get local tasks for user %d: %v", userID, err))
		return
	}

	// Build maps for comparison
	todoistByID := make(map[string]todoist.Task)
	for _, t := range todoistTasks {
		todoistByID[t.ID] = t
	}

	localByTodoistID := make(map[string]*domain.Task)
	localWithoutTodoist := make([]*domain.Task, 0)
	for _, t := range localTasks {
		if t.TodoistID != "" {
			localByTodoistID[t.TodoistID] = t
		} else {
			localWithoutTodoist = append(localWithoutTodoist, t)
		}
	}

	// 1. Sync FROM Todoist (new tasks in Todoist → create locally)
	for _, tt := range todoistTasks {
		if _, exists := localByTodoistID[tt.ID]; !exists {
			// Check if it's a task we created (has our label)
			if s.isFamilyBotTask(&tt) {
				continue // Skip tasks we created, they should have local link
			}

			// New task from Todoist - create locally
			task := s.todoistToLocalForUser(&tt, userID)
			if err := s.storage.CreateTask(task); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create local from todoist %s: %v", tt.ID, err))
			} else {
				result.FromTodoist.Added++
			}
		} else {
			// Task exists locally - check for updates from Todoist
			local := localByTodoistID[tt.ID]
			if s.needsUpdateFromTodoist(local, &tt) {
				s.updateLocalFromTodoist(local, &tt)
				if err := s.storage.UpdateTask(local); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update local from todoist %s: %v", tt.ID, err))
				} else {
					result.FromTodoist.Updated++
				}
			}
		}
	}

	// 2. Sync TO Todoist (local tasks without TodoistID → create in Todoist)
	for _, local := range localWithoutTodoist {
		// Skip repeating tasks - they don't make sense in Todoist
		if local.IsRepeating() {
			continue
		}

		req := s.localToTodoistForUser(local, userID)
		tt, err := s.client.CreateTask(req)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("create todoist from local %d: %v", local.ID, err))
		} else {
			// Update local task with Todoist ID
			if err := s.storage.UpdateTaskTodoistID(local.ID, tt.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update local todoist_id %d: %v", local.ID, err))
			} else {
				result.ToTodoist.Added++
			}
		}
	}

	// 3. Handle completed tasks in Todoist (mark local as done)
	for todoistID, local := range localByTodoistID {
		if _, exists := todoistByID[todoistID]; !exists {
			// Task was completed or deleted in Todoist
			if local.DoneAt == nil {
				now := time.Now()
				local.DoneAt = &now
				if err := s.storage.UpdateTask(local); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("mark done %d: %v", local.ID, err))
				} else {
					result.FromTodoist.Deleted++
				}
			}
		}
	}
}

// SyncTaskToTodoist creates or updates a task in Todoist
func (s *TodoistService) SyncTaskToTodoist(task *domain.Task) error {
	if !s.IsConfigured() {
		return nil
	}

	// Skip repeating tasks
	if task.IsRepeating() {
		return nil
	}

	if task.TodoistID != "" {
		// Update existing Todoist task
		req := &todoist.UpdateTaskRequest{
			Content: &task.Title,
		}
		if task.DueDate != nil {
			dueStr := task.DueDate.Format("2006-01-02")
			req.DueDate = &dueStr
		}
		priority := s.priorityToTodoist(task.Priority)
		req.Priority = &priority

		return s.client.UpdateTask(task.TodoistID, req)
	}

	// Create new Todoist task (using user-specific section)
	req := s.localToTodoistForUser(task, task.UserID)
	tt, err := s.client.CreateTask(req)
	if err != nil {
		return err
	}

	// Update local task with Todoist ID
	return s.storage.UpdateTaskTodoistID(task.ID, tt.ID)
}

// CompleteTaskInTodoist marks a task as complete in Todoist
func (s *TodoistService) CompleteTaskInTodoist(task *domain.Task) error {
	if !s.IsConfigured() || task.TodoistID == "" {
		return nil
	}

	return s.client.CloseTask(task.TodoistID)
}

// DeleteTaskFromTodoist deletes a task from Todoist
func (s *TodoistService) DeleteTaskFromTodoist(todoistID string) error {
	if !s.IsConfigured() || todoistID == "" {
		return nil
	}

	return s.client.DeleteTask(todoistID)
}

// todoistToLocal converts a Todoist task to local domain.Task (for owner)
func (s *TodoistService) todoistToLocal(tt *todoist.Task) *domain.Task {
	return s.todoistToLocalForUser(tt, s.ownerUserID)
}

// todoistToLocalForUser converts a Todoist task to local domain.Task for a specific user
func (s *TodoistService) todoistToLocalForUser(tt *todoist.Task, userID int64) *domain.Task {
	task := &domain.Task{
		UserID:    userID,
		Title:     tt.Content,
		Priority:  s.priorityFromTodoist(tt.Priority),
		IsShared:  false, // Don't auto-share Todoist tasks to avoid duplication
		TodoistID: tt.ID,
		CreatedAt: time.Now(),
	}

	if tt.Description != "" {
		task.Description = tt.Description
	}

	if tt.Due != nil {
		if tt.Due.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, tt.Due.DateTime); err == nil {
				task.DueDate = &t
			}
		} else if tt.Due.Date != "" {
			if t, err := time.Parse("2006-01-02", tt.Due.Date); err == nil {
				task.DueDate = &t
			}
		}
	}

	return task
}

// localToTodoist converts a local task to Todoist create request (for owner)
func (s *TodoistService) localToTodoist(task *domain.Task) *todoist.CreateTaskRequest {
	return s.localToTodoistForUser(task, s.ownerUserID)
}

// localToTodoistForUser converts a local task to Todoist create request for a specific user
func (s *TodoistService) localToTodoistForUser(task *domain.Task, userID int64) *todoist.CreateTaskRequest {
	req := &todoist.CreateTaskRequest{
		Content:  task.Title,
		Priority: s.priorityToTodoist(task.Priority),
		Labels:   []string{"familybot"}, // Mark tasks from FamilyBot
	}

	// Set section based on user
	if userID == s.ownerUserID {
		if sectionID := s.client.GetSectionID(); sectionID != "" {
			req.SectionID = sectionID
		}
	} else if userID == s.partnerUserID {
		if sectionID := s.client.GetPartnerSectionID(); sectionID != "" {
			req.SectionID = sectionID
		}
	}

	if task.Description != "" {
		req.Description = task.Description
	}

	if task.DueDate != nil {
		req.DueDate = task.DueDate.Format("2006-01-02")
	}

	return req
}

// priorityFromTodoist converts Todoist priority (1-4, where 4 is urgent) to domain.Priority
func (s *TodoistService) priorityFromTodoist(p int) domain.Priority {
	switch p {
	case 4:
		return domain.PriorityUrgent
	case 3:
		return domain.PriorityWeek
	default:
		return domain.PrioritySomeday
	}
}

// priorityToTodoist converts domain.Priority to Todoist priority (1-4)
func (s *TodoistService) priorityToTodoist(p domain.Priority) int {
	switch p {
	case domain.PriorityUrgent:
		return 4
	case domain.PriorityWeek:
		return 3
	default:
		return 1
	}
}

// isFamilyBotTask checks if a Todoist task was created by FamilyBot
func (s *TodoistService) isFamilyBotTask(tt *todoist.Task) bool {
	for _, label := range tt.Labels {
		if label == "familybot" {
			return true
		}
	}
	return false
}

// needsUpdateFromTodoist checks if local task needs update from Todoist
func (s *TodoistService) needsUpdateFromTodoist(local *domain.Task, tt *todoist.Task) bool {
	if local.Title != tt.Content {
		return true
	}

	// Check due date
	if tt.Due != nil {
		var todoistDue time.Time
		if tt.Due.DateTime != "" {
			todoistDue, _ = time.Parse(time.RFC3339, tt.Due.DateTime)
		} else if tt.Due.Date != "" {
			todoistDue, _ = time.Parse("2006-01-02", tt.Due.Date)
		}

		if local.DueDate == nil || !sameDate(local.DueDate, &todoistDue) {
			return true
		}
	} else if local.DueDate != nil {
		return true
	}

	return false
}

// updateLocalFromTodoist updates local task from Todoist data
func (s *TodoistService) updateLocalFromTodoist(local *domain.Task, tt *todoist.Task) {
	local.Title = tt.Content
	local.Description = tt.Description
	local.Priority = s.priorityFromTodoist(tt.Priority)

	if tt.Due != nil {
		var due time.Time
		if tt.Due.DateTime != "" {
			due, _ = time.Parse(time.RFC3339, tt.Due.DateTime)
		} else if tt.Due.Date != "" {
			due, _ = time.Parse("2006-01-02", tt.Due.Date)
		}
		if !due.IsZero() {
			local.DueDate = &due
		}
	} else {
		local.DueDate = nil
	}
}

// sameDate checks if two dates are the same day
func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// GetProjects returns available Todoist projects
func (s *TodoistService) GetProjects() ([]todoist.Project, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("Todoist not configured")
	}
	return s.client.GetProjects()
}

// GetSections returns sections for a project
func (s *TodoistService) GetSections(projectID string) ([]todoist.Section, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("Todoist not configured")
	}
	return s.client.GetSections(projectID)
}

// FormatSyncResult formats sync result for display
func (s *TodoistService) FormatSyncResult(result *TodoistSyncResult) string {
	var sb strings.Builder
	sb.WriteString("✅ Синхронизация с Todoist завершена!\n\n")

	sb.WriteString("<b>📥 Из Todoist:</b>\n")
	sb.WriteString(fmt.Sprintf("  ➕ Добавлено: %d\n", result.FromTodoist.Added))
	sb.WriteString(fmt.Sprintf("  🔄 Обновлено: %d\n", result.FromTodoist.Updated))
	sb.WriteString(fmt.Sprintf("  ✓ Завершено: %d\n", result.FromTodoist.Deleted))

	sb.WriteString(fmt.Sprintf("\n<b>📤 В Todoist:</b>\n"))
	sb.WriteString(fmt.Sprintf("  ➕ Добавлено: %d\n", result.ToTodoist.Added))
	sb.WriteString(fmt.Sprintf("  🔄 Обновлено: %d\n", result.ToTodoist.Updated))

	if len(result.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ Ошибок: %d", len(result.Errors)))
		for _, e := range result.Errors {
			log.Printf("Todoist sync error: %s", e)
		}
	}

	return sb.String()
}
