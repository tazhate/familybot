package domain

import "time"

type Priority string

const (
	PriorityUrgent  Priority = "urgent"
	PriorityWeek    Priority = "week"
	PrioritySomeday Priority = "someday"
)

type Task struct {
	ID          int64
	UserID      int64
	AssignedTo  *int64
	Title       string
	Description string
	Priority    Priority
	IsShared    bool
	DueDate     *time.Time
	DoneAt      *time.Time
	CreatedAt   time.Time
}

func (t *Task) IsDone() bool {
	return t.DoneAt != nil
}

func (t *Task) PriorityEmoji() string {
	switch t.Priority {
	case PriorityUrgent:
		return "🔴"
	case PriorityWeek:
		return "🟡"
	case PrioritySomeday:
		return "🟢"
	default:
		return "⚪"
	}
}
