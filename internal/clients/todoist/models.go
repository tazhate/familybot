package todoist

import "time"

// Task represents a Todoist task
type Task struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id,omitempty"`
	SectionID    string    `json:"section_id,omitempty"`
	Content      string    `json:"content"`
	Description  string    `json:"description,omitempty"`
	Priority     int       `json:"priority,omitempty"` // 1 (normal) to 4 (urgent)
	Due          *Due      `json:"due,omitempty"`
	Labels       []string  `json:"labels,omitempty"`
	AssigneeID   string    `json:"assignee_id,omitempty"`
	IsCompleted  bool      `json:"is_completed,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	CreatorID    string    `json:"creator_id,omitempty"`
	CommentCount int       `json:"comment_count,omitempty"`
	URL          string    `json:"url,omitempty"`
}

// Due represents due date info
type Due struct {
	String      string `json:"string,omitempty"`      // Human readable
	Date        string `json:"date,omitempty"`        // YYYY-MM-DD
	DateTime    string `json:"datetime,omitempty"`    // RFC3339
	IsRecurring bool   `json:"is_recurring,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
}

// CreateTaskRequest for creating a new task
type CreateTaskRequest struct {
	Content     string   `json:"content"`
	Description string   `json:"description,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	SectionID   string   `json:"section_id,omitempty"`
	Priority    int      `json:"priority,omitempty"`
	DueString   string   `json:"due_string,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
	DueDatetime string   `json:"due_datetime,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// UpdateTaskRequest for updating a task
type UpdateTaskRequest struct {
	Content     *string  `json:"content,omitempty"`
	Description *string  `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	DueString   *string  `json:"due_string,omitempty"`
	DueDate     *string  `json:"due_date,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// Project represents a Todoist project
type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Color          string `json:"color,omitempty"`
	ParentID       string `json:"parent_id,omitempty"`
	Order          int    `json:"order,omitempty"`
	CommentCount   int    `json:"comment_count,omitempty"`
	IsShared       bool   `json:"is_shared,omitempty"`
	IsFavorite     bool   `json:"is_favorite,omitempty"`
	IsInboxProject bool   `json:"is_inbox_project,omitempty"`
	ViewStyle      string `json:"view_style,omitempty"`
	URL            string `json:"url,omitempty"`
}

// Section represents a Todoist section within a project
type Section struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Order     int    `json:"order,omitempty"`
}
