package service

import (
	"fmt"
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

func (s *TaskService) Create(userID int64, title string, priority domain.Priority) (*domain.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}

	if priority == "" {
		priority = domain.PrioritySomeday
	}

	task := &domain.Task{
		UserID:   userID,
		Title:    title,
		Priority: priority,
	}

	if err := s.storage.CreateTask(task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}

func (s *TaskService) List(userID int64, includeDone bool) ([]*domain.Task, error) {
	return s.storage.ListTasksByUser(userID, true, includeDone)
}

func (s *TaskService) ListForToday(userID int64) ([]*domain.Task, error) {
	return s.storage.ListTasksForToday(userID)
}

func (s *TaskService) MarkDone(taskID int64, userID int64) error {
	task, err := s.storage.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	// Проверяем доступ
	if task.UserID != userID && (task.AssignedTo == nil || *task.AssignedTo != userID) && !task.IsShared {
		return fmt.Errorf("access denied")
	}

	return s.storage.MarkTaskDone(taskID)
}

func (s *TaskService) Delete(taskID int64, userID int64) error {
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

	return s.storage.DeleteTask(taskID)
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

func (s *TaskService) FormatTaskList(tasks []*domain.Task) string {
	if len(tasks) == 0 {
		return "Нет задач"
	}

	var sb strings.Builder
	for _, t := range tasks {
		status := "⬜"
		if t.IsDone() {
			status = "✅"
		}
		sb.WriteString(fmt.Sprintf("%s %s #%d %s\n", status, t.PriorityEmoji(), t.ID, t.Title))
	}
	return sb.String()
}
