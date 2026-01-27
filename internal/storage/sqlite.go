package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"

	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

func New(dbPath string) (*Storage, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER UNIQUE NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'owner',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			assigned_to INTEGER,
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			priority TEXT DEFAULT 'someday',
			is_shared INTEGER DEFAULT 0,
			due_date DATETIME,
			done_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (assigned_to) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			schedule TEXT NOT NULL,
			params TEXT DEFAULT '{}',
			is_active INTEGER DEFAULT 1,
			last_sent DATETIME,
			next_run DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_done_at ON tasks(done_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reminders_user_id ON reminders(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_reminders_next_run ON reminders(next_run)`,
		// Persons table
		`CREATE TABLE IF NOT EXISTS persons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'contact',
			birthday DATE,
			notes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_persons_user_id ON persons(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_persons_birthday ON persons(birthday)`,
		// Add person_id to tasks
		`ALTER TABLE tasks ADD COLUMN person_id INTEGER REFERENCES persons(id)`,
		// Weekly schedule table
		`CREATE TABLE IF NOT EXISTS weekly_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			day_of_week INTEGER NOT NULL,
			time_start TEXT NOT NULL,
			time_end TEXT DEFAULT '',
			title TEXT NOT NULL,
			person_id INTEGER,
			reminder_before INTEGER DEFAULT 0,
			is_floating INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (person_id) REFERENCES persons(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_weekly_events_user_id ON weekly_events(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_weekly_events_day ON weekly_events(day_of_week)`,
		// Floating events support
		`ALTER TABLE weekly_events ADD COLUMN floating_days TEXT DEFAULT ''`,
		`ALTER TABLE weekly_events ADD COLUMN confirmed_day INTEGER`,
		`ALTER TABLE weekly_events ADD COLUMN confirmed_week INTEGER DEFAULT 0`,
		// Multi-chat support for tasks
		`ALTER TABLE tasks ADD COLUMN chat_id INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_chat_id ON tasks(chat_id)`,
		// Migrate existing tasks to personal chat (chat_id = user's telegram_id)
		`UPDATE tasks SET chat_id = (SELECT telegram_id FROM users WHERE users.id = tasks.user_id) WHERE chat_id = 0`,
		// Set default reminder_before for floating events that don't have one
		`UPDATE weekly_events SET reminder_before = 30 WHERE is_floating = 1 AND reminder_before = 0`,
		// Reminder tracking for urgent tasks
		`ALTER TABLE tasks ADD COLUMN reminder_count INTEGER DEFAULT 0`,
		`ALTER TABLE tasks ADD COLUMN last_reminded_at DATETIME`,
		`ALTER TABLE tasks ADD COLUMN snooze_until DATETIME`,
		// Autos table
		`CREATE TABLE IF NOT EXISTS autos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			year INTEGER DEFAULT 0,
			insurance_until DATE,
			maintenance_until DATE,
			notes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_autos_user_id ON autos(user_id)`,
		// Repeating tasks
		`ALTER TABLE tasks ADD COLUMN repeat_type TEXT DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN repeat_time TEXT DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN repeat_week_num INTEGER DEFAULT 0`,
		// Checklists
		`CREATE TABLE IF NOT EXISTS checklists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			items TEXT DEFAULT '[]',
			person_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (person_id) REFERENCES persons(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checklists_user_id ON checklists(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_checklists_title ON checklists(title)`,
		// Person telegram link
		`ALTER TABLE persons ADD COLUMN telegram_id INTEGER`,
		// Task reminders (напоминания привязанные к задачам)
		`CREATE TABLE IF NOT EXISTS task_reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			remind_before INTEGER NOT NULL,
			sent_at DATETIME,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_reminders_task_id ON task_reminders(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_reminders_sent ON task_reminders(sent_at)`,
		// Shared weekly events
		`ALTER TABLE weekly_events ADD COLUMN is_shared INTEGER DEFAULT 0`,
		// Link checklist to weekly event
		`ALTER TABLE weekly_events ADD COLUMN checklist_id INTEGER REFERENCES checklists(id)`,
		// Trackable weekly events (create tasks that can be marked done)
		`ALTER TABLE weekly_events ADD COLUMN is_trackable INTEGER DEFAULT 0`,
		// Calendar events (synced from Apple Calendar)
		`CREATE TABLE IF NOT EXISTS calendar_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			caldav_uid TEXT UNIQUE,
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			location TEXT DEFAULT '',
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			all_day INTEGER DEFAULT 0,
			is_shared INTEGER DEFAULT 1,
			synced_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_calendar_events_start ON calendar_events(start_time)`,
		`CREATE INDEX IF NOT EXISTS idx_calendar_events_caldav ON calendar_events(caldav_uid)`,
		`CREATE INDEX IF NOT EXISTS idx_calendar_events_user ON calendar_events(user_id)`,
		// Todoist sync
		`ALTER TABLE tasks ADD COLUMN todoist_id TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_todoist ON tasks(todoist_id)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			// Ignore "duplicate column" errors for ALTER TABLE
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("exec migration: %w", err)
			}
		}
	}
	return nil
}

// === Users ===

func (s *Storage) CreateUser(u *domain.User) error {
	res, err := s.db.Exec(
		`INSERT INTO users (telegram_id, name, role) VALUES (?, ?, ?)`,
		u.TelegramID, u.Name, u.Role,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = id
	u.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetUserByTelegramID(telegramID int64) (*domain.User, error) {
	u := &domain.User{}
	err := s.db.QueryRow(
		`SELECT id, telegram_id, name, role, created_at FROM users WHERE telegram_id = ?`,
		telegramID,
	).Scan(&u.ID, &u.TelegramID, &u.Name, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Storage) GetUserByID(id int64) (*domain.User, error) {
	u := &domain.User{}
	err := s.db.QueryRow(
		`SELECT id, telegram_id, name, role, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.TelegramID, &u.Name, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// ListUsers returns all users
func (s *Storage) ListUsers() ([]*domain.User, error) {
	rows, err := s.db.Query(`SELECT id, telegram_id, name, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Name, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetUser is an alias for GetUserByID
func (s *Storage) GetUser(id int64) (*domain.User, error) {
	return s.GetUserByID(id)
}

// GetUserByName searches for a user by name (case insensitive)
func (s *Storage) GetUserByName(name string) (*domain.User, error) {
	u := &domain.User{}
	err := s.db.QueryRow(
		`SELECT id, telegram_id, name, role, created_at FROM users WHERE LOWER(name) LIKE LOWER(?)`,
		"%"+name+"%",
	).Scan(&u.ID, &u.TelegramID, &u.Name, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// === Tasks ===

func (s *Storage) CreateTask(t *domain.Task) error {
	res, err := s.db.Exec(
		`INSERT INTO tasks (user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, repeat_type, repeat_time, repeat_week_num, todoist_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.ChatID, t.AssignedTo, t.PersonID, t.Title, t.Description, t.Priority, t.IsShared, t.DueDate, t.RepeatType, t.RepeatTime, t.RepeatWeekNum, t.TodoistID,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	t.CreatedAt = time.Now()
	return nil
}

// TaskExistsForEventToday checks if task with given title exists for user with due_date = today
func (s *Storage) TaskExistsForEventToday(userID int64, title string, todayStart time.Time) (bool, error) {
	todayEnd := todayStart.Add(24 * time.Hour)
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE user_id = ? AND title = ? AND due_date >= ? AND due_date < ? AND done_at IS NULL`,
		userID, title, todayStart, todayEnd,
	).Scan(&count)
	return count > 0, err
}

func (s *Storage) GetTask(id int64) (*domain.Task, error) {
	t := &domain.Task{}
	err := s.db.QueryRow(
		`SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		 FROM tasks WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// GetTaskByTodoistID returns a task by its Todoist ID
func (s *Storage) GetTaskByTodoistID(todoistID string) (*domain.Task, error) {
	if todoistID == "" {
		return nil, nil
	}
	t := &domain.Task{}
	err := s.db.QueryRow(
		`SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		 FROM tasks WHERE todoist_id = ?`,
		todoistID,
	).Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// UpdateTaskTodoistID sets the Todoist ID for a task
func (s *Storage) UpdateTaskTodoistID(taskID int64, todoistID string) error {
	_, err := s.db.Exec(`UPDATE tasks SET todoist_id = ? WHERE id = ?`, todoistID, taskID)
	return err
}

func (s *Storage) ListTasksByUser(userID int64, includeShared bool, includeDone bool) ([]*domain.Task, error) {
	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks WHERE (user_id = ? OR assigned_to = ?`
	if includeShared {
		query += ` OR is_shared = 1`
	}
	query += `)`
	if !includeDone {
		query += ` AND done_at IS NULL`
	}
	query += ` ORDER BY
		CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		created_at DESC`

	rows, err := s.db.Query(query, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListTasksByChat returns tasks for a specific chat context (including shared tasks)
func (s *Storage) ListTasksByChat(chatID int64, includeDone bool) ([]*domain.Task, error) {
	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks WHERE (chat_id = ? OR is_shared = 1)`
	if !includeDone {
		query += ` AND done_at IS NULL`
	}
	query += ` ORDER BY
		CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		created_at DESC`

	rows, err := s.db.Query(query, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListSharedTasks returns all shared tasks (is_shared = true)
func (s *Storage) ListSharedTasks(includeDone bool) ([]*domain.Task, error) {
	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks WHERE is_shared = 1`
	if !includeDone {
		query += ` AND done_at IS NULL`
	}
	query += ` ORDER BY
		CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateTaskShared updates the is_shared flag for a task
func (s *Storage) UpdateTaskShared(taskID int64, isShared bool) error {
	_, err := s.db.Exec(`UPDATE tasks SET is_shared = ? WHERE id = ?`, isShared, taskID)
	return err
}

func (s *Storage) ListTasksForToday(userID int64) ([]*domain.Task, error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	// Show tasks that are:
	// 1. Due today (any priority)
	// 2. Urgent with no due_date
	// 3. Urgent with due_date today or in the past (overdue)
	rows, err := s.db.Query(
		`SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		 FROM tasks
		 WHERE (user_id = ? OR assigned_to = ? OR is_shared = 1)
		   AND done_at IS NULL
		   AND (
		     (due_date >= ? AND due_date < ?)
		     OR (priority = 'urgent' AND (due_date IS NULL OR due_date < ?))
		   )
		 ORDER BY
		   CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		   due_date ASC`,
		userID, userID, today, tomorrow, tomorrow,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListTasksForTodayByChat returns today's tasks for a specific chat (including shared)
func (s *Storage) ListTasksForTodayByChat(chatID int64) ([]*domain.Task, error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	// Show tasks that are:
	// 1. Due today (any priority)
	// 2. Urgent with no due_date
	// 3. Urgent with due_date today or in the past (overdue)
	rows, err := s.db.Query(
		`SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		 FROM tasks
		 WHERE (chat_id = ? OR is_shared = 1)
		   AND done_at IS NULL
		   AND (
		     (due_date >= ? AND due_date < ?)
		     OR (priority = 'urgent' AND (due_date IS NULL OR due_date < ?))
		   )
		 ORDER BY
		   CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		   due_date ASC`,
		chatID, today, tomorrow, tomorrow,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListTasksByPerson returns tasks linked to a specific person
func (s *Storage) ListTasksByPerson(personID int64, includeDone bool) ([]*domain.Task, error) {
	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks WHERE person_id = ?`
	if !includeDone {
		query += ` AND done_at IS NULL`
	}
	query += ` ORDER BY CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END, created_at DESC`

	rows, err := s.db.Query(query, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateTaskAssignment assigns a task to a user
func (s *Storage) UpdateTaskAssignment(taskID int64, assignedTo *int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET assigned_to = ? WHERE id = ?`, assignedTo, taskID)
	return err
}

// UpdateTaskPerson links a task to a person
func (s *Storage) UpdateTaskPerson(taskID int64, personID *int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET person_id = ? WHERE id = ?`, personID, taskID)
	return err
}

func (s *Storage) MarkTaskDone(id int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET done_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

func (s *Storage) DeleteTask(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

// UpdateTask updates task fields
func (s *Storage) UpdateTask(t *domain.Task) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET title = ?, priority = ?, due_date = ?, person_id = ?, assigned_to = ? WHERE id = ?`,
		t.Title, t.Priority, t.DueDate, t.PersonID, t.AssignedTo, t.ID,
	)
	return err
}

// UpdateTaskTitle updates only the title
func (s *Storage) UpdateTaskTitle(taskID int64, title string) error {
	_, err := s.db.Exec(`UPDATE tasks SET title = ? WHERE id = ?`, title, taskID)
	return err
}

// UpdateTaskPriority updates only the priority
func (s *Storage) UpdateTaskPriority(taskID int64, priority domain.Priority) error {
	_, err := s.db.Exec(`UPDATE tasks SET priority = ? WHERE id = ?`, priority, taskID)
	return err
}

// UpdateTaskDueDate updates only the due date
func (s *Storage) UpdateTaskDueDate(taskID int64, dueDate *time.Time) error {
	_, err := s.db.Exec(`UPDATE tasks SET due_date = ? WHERE id = ?`, dueDate, taskID)
	return err
}

// ListUrgentTasksForReminder returns urgent tasks that need a reminder
// Criteria: priority=urgent, not done, created > 2h ago, reminder_count < 3,
// (snooze_until is null or past), (last_reminded_at is null or > 2h ago)
func (s *Storage) ListUrgentTasksForReminder() ([]*domain.Task, error) {
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	now := time.Now()

	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks
		WHERE priority = 'urgent'
		AND done_at IS NULL
		AND created_at < ?
		AND reminder_count < 3
		AND (snooze_until IS NULL OR snooze_until < ?)
		AND (last_reminded_at IS NULL OR last_reminded_at < ?)`

	rows, err := s.db.Query(query, twoHoursAgo, now, twoHoursAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateTaskReminder increments reminder count and updates last_reminded_at
func (s *Storage) UpdateTaskReminder(taskID int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET reminder_count = reminder_count + 1, last_reminded_at = ? WHERE id = ?`, time.Now(), taskID)
	return err
}

// SnoozeTask sets the snooze_until time for a task
func (s *Storage) SnoozeTask(taskID int64, until time.Time) error {
	_, err := s.db.Exec(`UPDATE tasks SET snooze_until = ? WHERE id = ?`, until, taskID)
	return err
}

// ListRepeatingTasksByTime returns repeating tasks with specified repeat_time
// that are not done and not snoozed
func (s *Storage) ListRepeatingTasksByTime(repeatTime string) ([]*domain.Task, error) {
	now := time.Now()

	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks
		WHERE repeat_time = ?
		AND repeat_type != ''
		AND done_at IS NULL
		AND (snooze_until IS NULL OR snooze_until < ?)`

	rows, err := s.db.Query(query, repeatTime, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// === Reminders ===

func (s *Storage) CreateReminder(r *domain.Reminder) error {
	res, err := s.db.Exec(
		`INSERT INTO reminders (user_id, title, type, schedule, params, is_active, next_run)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.Title, r.Type, r.Schedule, r.Params, r.IsActive, r.NextRun,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	r.ID = id
	r.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetReminder(id int64) (*domain.Reminder, error) {
	r := &domain.Reminder{}
	err := s.db.QueryRow(
		`SELECT id, user_id, title, type, schedule, params, is_active, last_sent, next_run, created_at
		 FROM reminders WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.UserID, &r.Title, &r.Type, &r.Schedule, &r.Params, &r.IsActive, &r.LastSent, &r.NextRun, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func (s *Storage) ListRemindersByUser(userID int64) ([]*domain.Reminder, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, title, type, schedule, params, is_active, last_sent, next_run, created_at
		 FROM reminders WHERE user_id = ? ORDER BY next_run ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*domain.Reminder
	for rows.Next() {
		r := &domain.Reminder{}
		if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.Type, &r.Schedule, &r.Params, &r.IsActive, &r.LastSent, &r.NextRun, &r.CreatedAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	return reminders, nil
}

func (s *Storage) ListDueReminders(now time.Time) ([]*domain.Reminder, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, title, type, schedule, params, is_active, last_sent, next_run, created_at
		 FROM reminders WHERE is_active = 1 AND next_run <= ?`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*domain.Reminder
	for rows.Next() {
		r := &domain.Reminder{}
		if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.Type, &r.Schedule, &r.Params, &r.IsActive, &r.LastSent, &r.NextRun, &r.CreatedAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	return reminders, nil
}

func (s *Storage) UpdateReminderNextRun(id int64, lastSent, nextRun time.Time) error {
	_, err := s.db.Exec(`UPDATE reminders SET last_sent = ?, next_run = ? WHERE id = ?`, lastSent, nextRun, id)
	return err
}

func (s *Storage) DeleteReminder(id int64) error {
	_, err := s.db.Exec(`DELETE FROM reminders WHERE id = ?`, id)
	return err
}

// UpdateReminder updates a reminder's title, schedule, and next_run
func (s *Storage) UpdateReminder(r *domain.Reminder) error {
	_, err := s.db.Exec(
		`UPDATE reminders SET title = ?, schedule = ?, next_run = ? WHERE id = ?`,
		r.Title, r.Schedule, r.NextRun, r.ID,
	)
	return err
}

// UpdateReminderTitle updates only the title
func (s *Storage) UpdateReminderTitle(id int64, title string) error {
	_, err := s.db.Exec(`UPDATE reminders SET title = ? WHERE id = ?`, title, id)
	return err
}

// === Persons ===

func (s *Storage) CreatePerson(p *domain.Person) error {
	res, err := s.db.Exec(
		`INSERT INTO persons (user_id, telegram_id, name, role, birthday, notes) VALUES (?, ?, ?, ?, ?, ?)`,
		p.UserID, p.TelegramID, p.Name, p.Role, p.Birthday, p.Notes,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	p.ID = id
	p.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetPerson(id int64) (*domain.Person, error) {
	p := &domain.Person{}
	err := s.db.QueryRow(
		`SELECT id, user_id, telegram_id, name, role, birthday, notes, created_at FROM persons WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.UserID, &p.TelegramID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Storage) GetPersonByName(userID int64, name string) (*domain.Person, error) {
	// SQLite LOWER() doesn't work with Cyrillic, so we fetch all and compare in Go
	persons, err := s.ListPersonsByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, p := range persons {
		if strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return nil, nil
}

func (s *Storage) ListPersonsByUser(userID int64) ([]*domain.Person, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, telegram_id, name, role, birthday, notes, created_at
		 FROM persons WHERE user_id = ? ORDER BY name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []*domain.Person
	for rows.Next() {
		p := &domain.Person{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.TelegramID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, nil
}

func (s *Storage) ListPersonsWithBirthday(userID int64) ([]*domain.Person, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, telegram_id, name, role, birthday, notes, created_at
		 FROM persons WHERE user_id = ? AND birthday IS NOT NULL ORDER BY
		 strftime('%m-%d', birthday) ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []*domain.Person
	for rows.Next() {
		p := &domain.Person{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.TelegramID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, nil
}

func (s *Storage) ListUpcomingBirthdays(userID int64, days int) ([]*domain.Person, error) {
	// Get persons whose birthday is within the next N days
	rows, err := s.db.Query(
		`SELECT id, user_id, telegram_id, name, role, birthday, notes, created_at
		 FROM persons
		 WHERE user_id = ? AND birthday IS NOT NULL
		 ORDER BY
		   CASE
		     WHEN strftime('%m-%d', birthday) >= strftime('%m-%d', 'now')
		     THEN strftime('%m-%d', birthday)
		     ELSE strftime('%m-%d', birthday, '+1 year')
		   END ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []*domain.Person
	for rows.Next() {
		p := &domain.Person{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.TelegramID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		// Filter by days until birthday
		daysUntil := p.DaysUntilBirthday()
		if daysUntil >= 0 && daysUntil <= days {
			persons = append(persons, p)
		}
	}
	return persons, nil
}

func (s *Storage) UpdatePerson(p *domain.Person) error {
	_, err := s.db.Exec(
		`UPDATE persons SET telegram_id = ?, name = ?, role = ?, birthday = ?, notes = ? WHERE id = ?`,
		p.TelegramID, p.Name, p.Role, p.Birthday, p.Notes, p.ID,
	)
	return err
}

// UpdatePersonTelegramID links a person to a Telegram user
func (s *Storage) UpdatePersonTelegramID(personID int64, telegramID *int64) error {
	_, err := s.db.Exec(`UPDATE persons SET telegram_id = ? WHERE id = ?`, telegramID, personID)
	return err
}

// GetPersonByTelegramID finds a person linked to a Telegram ID
func (s *Storage) GetPersonByTelegramID(userID int64, telegramID int64) (*domain.Person, error) {
	p := &domain.Person{}
	err := s.db.QueryRow(
		`SELECT id, user_id, telegram_id, name, role, birthday, notes, created_at FROM persons WHERE user_id = ? AND telegram_id = ?`,
		userID, telegramID,
	).Scan(&p.ID, &p.UserID, &p.TelegramID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Storage) DeletePerson(id int64) error {
	_, err := s.db.Exec(`DELETE FROM persons WHERE id = ?`, id)
	return err
}

// === Weekly Events ===

func (s *Storage) CreateWeeklyEvent(e *domain.WeeklyEvent) error {
	res, err := s.db.Exec(
		`INSERT INTO weekly_events (user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.DayOfWeek, e.TimeStart, e.TimeEnd, e.Title, e.PersonID, e.ChecklistID, e.ReminderBefore, e.IsFloating, e.FloatingDays, e.ConfirmedDay, e.ConfirmedWeek, e.IsShared, e.IsTrackable,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	e.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetWeeklyEvent(id int64) (*domain.WeeklyEvent, error) {
	e := &domain.WeeklyEvent{}
	err := s.db.QueryRow(
		`SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE id = ?`,
		id,
	).Scan(&e.ID, &e.UserID, &e.DayOfWeek, &e.TimeStart, &e.TimeEnd, &e.Title, &e.PersonID, &e.ChecklistID, &e.ReminderBefore, &e.IsFloating, &e.FloatingDays, &e.ConfirmedDay, &e.ConfirmedWeek, &e.IsShared, &e.IsTrackable, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func (s *Storage) ListWeeklyEventsByUser(userID int64, includeShared bool) ([]*domain.WeeklyEvent, error) {
	query := `SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE user_id = ?`
	if includeShared {
		query += ` OR is_shared = 1`
	}
	query += ` ORDER BY day_of_week, time_start`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.WeeklyEvent
	for rows.Next() {
		e := &domain.WeeklyEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.DayOfWeek, &e.TimeStart, &e.TimeEnd, &e.Title, &e.PersonID, &e.ChecklistID, &e.ReminderBefore, &e.IsFloating, &e.FloatingDays, &e.ConfirmedDay, &e.ConfirmedWeek, &e.IsShared, &e.IsTrackable, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *Storage) ListWeeklyEventsByDay(userID int64, dayOfWeek domain.Weekday, includeShared bool) ([]*domain.WeeklyEvent, error) {
	query := `SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE (user_id = ? OR is_shared = 1) AND day_of_week = ? ORDER BY time_start`
	if !includeShared {
		query = `SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE user_id = ? AND day_of_week = ? ORDER BY time_start`
	}

	rows, err := s.db.Query(query, userID, dayOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.WeeklyEvent
	for rows.Next() {
		e := &domain.WeeklyEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.DayOfWeek, &e.TimeStart, &e.TimeEnd, &e.Title, &e.PersonID, &e.ChecklistID, &e.ReminderBefore, &e.IsFloating, &e.FloatingDays, &e.ConfirmedDay, &e.ConfirmedWeek, &e.IsShared, &e.IsTrackable, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *Storage) DeleteWeeklyEvent(id int64) error {
	_, err := s.db.Exec(`DELETE FROM weekly_events WHERE id = ?`, id)
	return err
}

// UpdateWeeklyEvent updates a weekly event
func (s *Storage) UpdateWeeklyEvent(e *domain.WeeklyEvent) error {
	_, err := s.db.Exec(
		`UPDATE weekly_events SET day_of_week = ?, time_start = ?, time_end = ?, title = ?, reminder_before = ? WHERE id = ?`,
		e.DayOfWeek, e.TimeStart, e.TimeEnd, e.Title, e.ReminderBefore, e.ID,
	)
	return err
}

// UpdateWeeklyEventConfirmedDay sets the confirmed day for a floating event
func (s *Storage) UpdateWeeklyEventConfirmedDay(id int64, confirmedDay *int, confirmedWeek int) error {
	_, err := s.db.Exec(
		`UPDATE weekly_events SET confirmed_day = ?, confirmed_week = ? WHERE id = ?`,
		confirmedDay, confirmedWeek, id,
	)
	return err
}

// ListFloatingEvents returns all floating events for a user
func (s *Storage) ListFloatingEvents(userID int64) ([]*domain.WeeklyEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE user_id = ? AND is_floating = 1 ORDER BY time_start`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.WeeklyEvent
	for rows.Next() {
		e := &domain.WeeklyEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.DayOfWeek, &e.TimeStart, &e.TimeEnd, &e.Title, &e.PersonID, &e.ChecklistID, &e.ReminderBefore, &e.IsFloating, &e.FloatingDays, &e.ConfirmedDay, &e.ConfirmedWeek, &e.IsShared, &e.IsTrackable, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// ListEventsWithReminders returns all events with reminder_before > 0
func (s *Storage) ListEventsWithReminders() ([]*domain.WeeklyEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, day_of_week, time_start, time_end, title, person_id, checklist_id, reminder_before, is_floating, floating_days, confirmed_day, confirmed_week, is_shared, is_trackable, created_at
		 FROM weekly_events WHERE reminder_before > 0 ORDER BY day_of_week, time_start`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.WeeklyEvent
	for rows.Next() {
		e := &domain.WeeklyEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.DayOfWeek, &e.TimeStart, &e.TimeEnd, &e.Title, &e.PersonID, &e.ChecklistID, &e.ReminderBefore, &e.IsFloating, &e.FloatingDays, &e.ConfirmedDay, &e.ConfirmedWeek, &e.IsShared, &e.IsTrackable, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// UpdateWeeklyEventChecklist links a checklist to a weekly event
func (s *Storage) UpdateWeeklyEventChecklist(eventID int64, checklistID *int64) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET checklist_id = ? WHERE id = ?`, checklistID, eventID)
	return err
}

// UpdateWeeklyEventShared updates the is_shared flag for a weekly event
func (s *Storage) UpdateWeeklyEventShared(eventID int64, isShared bool) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET is_shared = ? WHERE id = ?`, isShared, eventID)
	return err
}

// UpdateWeeklyEventTrackable updates the is_trackable flag for a weekly event
func (s *Storage) UpdateWeeklyEventTrackable(eventID int64, isTrackable bool) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET is_trackable = ? WHERE id = ?`, isTrackable, eventID)
	return err
}

// UpdateWeeklyEventTitle updates the title of a weekly event
func (s *Storage) UpdateWeeklyEventTitle(eventID int64, title string) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET title = ? WHERE id = ?`, title, eventID)
	return err
}

// UpdateWeeklyEventDay updates the day of week for a weekly event
func (s *Storage) UpdateWeeklyEventDay(eventID int64, day domain.Weekday) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET day_of_week = ? WHERE id = ?`, day, eventID)
	return err
}

// UpdateWeeklyEventTime updates the time of a weekly event
func (s *Storage) UpdateWeeklyEventTime(eventID int64, timeStart, timeEnd string) error {
	_, err := s.db.Exec(`UPDATE weekly_events SET time_start = ?, time_end = ? WHERE id = ?`, timeStart, timeEnd, eventID)
	return err
}

// === Autos ===

func (s *Storage) CreateAuto(a *domain.Auto) error {
	res, err := s.db.Exec(
		`INSERT INTO autos (user_id, name, year, insurance_until, maintenance_until, notes)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.UserID, a.Name, a.Year, a.InsuranceUntil, a.MaintenanceUntil, a.Notes,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	a.ID = id
	a.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetAuto(id int64) (*domain.Auto, error) {
	a := &domain.Auto{}
	err := s.db.QueryRow(
		`SELECT id, user_id, name, year, insurance_until, maintenance_until, notes, created_at
		 FROM autos WHERE id = ?`,
		id,
	).Scan(&a.ID, &a.UserID, &a.Name, &a.Year, &a.InsuranceUntil, &a.MaintenanceUntil, &a.Notes, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Storage) ListAutosByUser(userID int64) ([]*domain.Auto, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, year, insurance_until, maintenance_until, notes, created_at
		 FROM autos WHERE user_id = ? ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var autos []*domain.Auto
	for rows.Next() {
		a := &domain.Auto{}
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Year, &a.InsuranceUntil, &a.MaintenanceUntil, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		autos = append(autos, a)
	}
	return autos, nil
}

func (s *Storage) UpdateAutoInsurance(id int64, until time.Time) error {
	_, err := s.db.Exec(`UPDATE autos SET insurance_until = ? WHERE id = ?`, until, id)
	return err
}

func (s *Storage) UpdateAutoMaintenance(id int64, until time.Time) error {
	_, err := s.db.Exec(`UPDATE autos SET maintenance_until = ? WHERE id = ?`, until, id)
	return err
}

func (s *Storage) DeleteAuto(id int64) error {
	_, err := s.db.Exec(`DELETE FROM autos WHERE id = ?`, id)
	return err
}

// ListAutosNeedingReminder returns autos with insurance or maintenance due within given days
func (s *Storage) ListAutosNeedingReminder(days int) ([]*domain.Auto, error) {
	deadline := time.Now().AddDate(0, 0, days)
	rows, err := s.db.Query(
		`SELECT id, user_id, name, year, insurance_until, maintenance_until, notes, created_at
		 FROM autos
		 WHERE insurance_until <= ? OR maintenance_until <= ?
		 ORDER BY insurance_until, maintenance_until`,
		deadline, deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var autos []*domain.Auto
	for rows.Next() {
		a := &domain.Auto{}
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Year, &a.InsuranceUntil, &a.MaintenanceUntil, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		autos = append(autos, a)
	}
	return autos, nil
}

// === Checklists ===

func (s *Storage) CreateChecklist(c *domain.Checklist) error {
	res, err := s.db.Exec(
		`INSERT INTO checklists (user_id, title, items, person_id) VALUES (?, ?, ?, ?)`,
		c.UserID, c.Title, c.ItemsJSON(), c.PersonID,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	c.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetChecklist(id int64) (*domain.Checklist, error) {
	c := &domain.Checklist{}
	var itemsJSON string
	err := s.db.QueryRow(
		`SELECT id, user_id, title, items, person_id, created_at FROM checklists WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.UserID, &c.Title, &itemsJSON, &c.PersonID, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ParseItemsJSON(itemsJSON)
	return c, nil
}

func (s *Storage) GetChecklistByTitle(userID int64, title string) (*domain.Checklist, error) {
	c := &domain.Checklist{}
	var itemsJSON string
	err := s.db.QueryRow(
		`SELECT id, user_id, title, items, person_id, created_at FROM checklists WHERE user_id = ? AND LOWER(title) = LOWER(?)`,
		userID, title,
	).Scan(&c.ID, &c.UserID, &c.Title, &itemsJSON, &c.PersonID, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ParseItemsJSON(itemsJSON)
	return c, nil
}

func (s *Storage) ListChecklistsByUser(userID int64) ([]*domain.Checklist, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, title, items, person_id, created_at FROM checklists WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checklists []*domain.Checklist
	for rows.Next() {
		c := &domain.Checklist{}
		var itemsJSON string
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &itemsJSON, &c.PersonID, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.ParseItemsJSON(itemsJSON)
		checklists = append(checklists, c)
	}
	return checklists, nil
}

func (s *Storage) UpdateChecklistItems(id int64, items []domain.ChecklistItem) error {
	c := &domain.Checklist{Items: items}
	_, err := s.db.Exec(`UPDATE checklists SET items = ? WHERE id = ?`, c.ItemsJSON(), id)
	return err
}

func (s *Storage) DeleteChecklist(id int64) error {
	_, err := s.db.Exec(`DELETE FROM checklists WHERE id = ?`, id)
	return err
}

// === Task History & Stats ===

// ListCompletedTasks returns completed tasks ordered by completion time
func (s *Storage) ListCompletedTasks(userID int64, limit int) ([]*domain.Task, error) {
	query := `SELECT id, user_id, chat_id, assigned_to, person_id, title, description, priority, is_shared, due_date, done_at, created_at, reminder_count, last_reminded_at, snooze_until, repeat_type, repeat_time, repeat_week_num, COALESCE(todoist_id, '')
		FROM tasks
		WHERE (user_id = ? OR assigned_to = ?)
		AND done_at IS NOT NULL
		ORDER BY done_at DESC
		LIMIT ?`

	rows, err := s.db.Query(query, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt, &t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// GetTaskStats returns task statistics for a user
func (s *Storage) GetTaskStats(userID int64, since time.Time) (completed int, created int, err error) {
	// Completed tasks since date
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE (user_id = ? OR assigned_to = ?) AND done_at >= ?`,
		userID, userID, since,
	).Scan(&completed)
	if err != nil {
		return 0, 0, err
	}

	// Created tasks since date
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE user_id = ? AND created_at >= ?`,
		userID, since,
	).Scan(&created)
	if err != nil {
		return 0, 0, err
	}

	return completed, created, nil
}

// GetPendingTaskCount returns count of pending tasks
func (s *Storage) GetPendingTaskCount(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE (user_id = ? OR assigned_to = ?) AND done_at IS NULL`,
		userID, userID,
	).Scan(&count)
	return count, err
}

// === Task Reminders ===

// CreateTaskReminder creates a reminder for a task
func (s *Storage) CreateTaskReminder(tr *domain.TaskReminder) error {
	res, err := s.db.Exec(
		`INSERT INTO task_reminders (task_id, remind_before) VALUES (?, ?)`,
		tr.TaskID, tr.RemindBefore,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	tr.ID = id
	return nil
}

// GetTaskReminders returns all reminders for a task
func (s *Storage) GetTaskReminders(taskID int64) ([]*domain.TaskReminder, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, remind_before, sent_at FROM task_reminders WHERE task_id = ? ORDER BY remind_before DESC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*domain.TaskReminder
	for rows.Next() {
		r := &domain.TaskReminder{}
		if err := rows.Scan(&r.ID, &r.TaskID, &r.RemindBefore, &r.SentAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	return reminders, nil
}

// DeleteTaskReminder deletes a task reminder
func (s *Storage) DeleteTaskReminder(id int64) error {
	_, err := s.db.Exec(`DELETE FROM task_reminders WHERE id = ?`, id)
	return err
}

// DeleteTaskRemindersByTask deletes all reminders for a task
func (s *Storage) DeleteTaskRemindersByTask(taskID int64) error {
	_, err := s.db.Exec(`DELETE FROM task_reminders WHERE task_id = ?`, taskID)
	return err
}

// MarkTaskReminderSent marks a reminder as sent
func (s *Storage) MarkTaskReminderSent(id int64) error {
	_, err := s.db.Exec(
		`UPDATE task_reminders SET sent_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	return err
}

// GetPendingTaskReminders returns task reminders that need to be sent
// A reminder is pending if:
// - sent_at IS NULL
// - task has due_date
// - task is not done
// - (due_date - remind_before minutes) <= now
func (s *Storage) GetPendingTaskReminders() ([]*domain.TaskReminder, []*domain.Task, error) {
	rows, err := s.db.Query(`
		SELECT tr.id, tr.task_id, tr.remind_before, tr.sent_at,
		       t.id, t.user_id, t.chat_id, t.assigned_to, t.person_id, t.title, t.description,
		       t.priority, t.is_shared, t.due_date, t.done_at, t.created_at,
		       t.reminder_count, t.last_reminded_at, t.snooze_until, t.repeat_type, t.repeat_time, t.repeat_week_num, COALESCE(t.todoist_id, '')
		FROM task_reminders tr
		JOIN tasks t ON tr.task_id = t.id
		WHERE tr.sent_at IS NULL
		  AND t.due_date IS NOT NULL
		  AND t.done_at IS NULL
		  AND datetime(t.due_date, '-' || tr.remind_before || ' minutes') <= datetime('now')
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var reminders []*domain.TaskReminder
	var tasks []*domain.Task
	for rows.Next() {
		r := &domain.TaskReminder{}
		t := &domain.Task{}
		if err := rows.Scan(
			&r.ID, &r.TaskID, &r.RemindBefore, &r.SentAt,
			&t.ID, &t.UserID, &t.ChatID, &t.AssignedTo, &t.PersonID, &t.Title, &t.Description,
			&t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt,
			&t.ReminderCount, &t.LastRemindedAt, &t.SnoozeUntil, &t.RepeatType, &t.RepeatTime, &t.RepeatWeekNum, &t.TodoistID,
		); err != nil {
			return nil, nil, err
		}
		reminders = append(reminders, r)
		tasks = append(tasks, t)
	}
	return reminders, tasks, nil
}

// === Calendar Events ===

// CreateCalendarEvent creates a new calendar event
func (s *Storage) CreateCalendarEvent(e *domain.CalendarEvent) error {
	now := time.Now()
	res, err := s.db.Exec(
		`INSERT INTO calendar_events (user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.CalDAVUID, e.Title, e.Description, e.Location, e.StartTime, e.EndTime, e.AllDay, e.IsShared, e.SyncedAt, now, now,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	e.CreatedAt = now
	e.UpdatedAt = now
	return nil
}

// GetCalendarEvent returns a calendar event by ID
func (s *Storage) GetCalendarEvent(id int64) (*domain.CalendarEvent, error) {
	e := &domain.CalendarEvent{}
	err := s.db.QueryRow(
		`SELECT id, user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at
		 FROM calendar_events WHERE id = ?`,
		id,
	).Scan(&e.ID, &e.UserID, &e.CalDAVUID, &e.Title, &e.Description, &e.Location, &e.StartTime, &e.EndTime, &e.AllDay, &e.IsShared, &e.SyncedAt, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// GetCalendarEventByCalDAVUID returns a calendar event by CalDAV UID
func (s *Storage) GetCalendarEventByCalDAVUID(uid string) (*domain.CalendarEvent, error) {
	e := &domain.CalendarEvent{}
	err := s.db.QueryRow(
		`SELECT id, user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at
		 FROM calendar_events WHERE caldav_uid = ?`,
		uid,
	).Scan(&e.ID, &e.UserID, &e.CalDAVUID, &e.Title, &e.Description, &e.Location, &e.StartTime, &e.EndTime, &e.AllDay, &e.IsShared, &e.SyncedAt, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// UpdateCalendarEvent updates an existing calendar event
func (s *Storage) UpdateCalendarEvent(e *domain.CalendarEvent) error {
	e.UpdatedAt = time.Now()
	_, err := s.db.Exec(
		`UPDATE calendar_events SET title = ?, description = ?, location = ?, start_time = ?, end_time = ?, all_day = ?, is_shared = ?, synced_at = ?, updated_at = ?
		 WHERE id = ?`,
		e.Title, e.Description, e.Location, e.StartTime, e.EndTime, e.AllDay, e.IsShared, e.SyncedAt, e.UpdatedAt, e.ID,
	)
	return err
}

// DeleteCalendarEvent deletes a calendar event by ID
func (s *Storage) DeleteCalendarEvent(id int64) error {
	_, err := s.db.Exec(`DELETE FROM calendar_events WHERE id = ?`, id)
	return err
}

// DeleteCalendarEventByCalDAVUID deletes a calendar event by CalDAV UID
func (s *Storage) DeleteCalendarEventByCalDAVUID(uid string) error {
	_, err := s.db.Exec(`DELETE FROM calendar_events WHERE caldav_uid = ?`, uid)
	return err
}

// ListCalendarEvents returns calendar events in a time range
func (s *Storage) ListCalendarEvents(userID int64, from, to time.Time, includeShared bool) ([]*domain.CalendarEvent, error) {
	query := `SELECT id, user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at
		FROM calendar_events
		WHERE start_time >= ? AND start_time < ?`
	if includeShared {
		query += ` AND (user_id = ? OR is_shared = 1)`
	} else {
		query += ` AND user_id = ?`
	}
	query += ` ORDER BY start_time ASC`

	rows, err := s.db.Query(query, from, to, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.CalendarEvent
	for rows.Next() {
		e := &domain.CalendarEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.CalDAVUID, &e.Title, &e.Description, &e.Location, &e.StartTime, &e.EndTime, &e.AllDay, &e.IsShared, &e.SyncedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// ListCalendarEventsToday returns today's calendar events
func (s *Storage) ListCalendarEventsToday(userID int64, includeShared bool) ([]*domain.CalendarEvent, error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)
	return s.ListCalendarEvents(userID, today, tomorrow, includeShared)
}

// ListCalendarEventsWeek returns this week's calendar events
func (s *Storage) ListCalendarEventsWeek(userID int64, includeShared bool) ([]*domain.CalendarEvent, error) {
	today := time.Now().Truncate(24 * time.Hour)
	weekLater := today.Add(7 * 24 * time.Hour)
	return s.ListCalendarEvents(userID, today, weekLater, includeShared)
}

// ListAllCalendarEvents returns all calendar events (for sync purposes)
func (s *Storage) ListAllCalendarEvents() ([]*domain.CalendarEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at
		 FROM calendar_events ORDER BY start_time ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.CalendarEvent
	for rows.Next() {
		e := &domain.CalendarEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.CalDAVUID, &e.Title, &e.Description, &e.Location, &e.StartTime, &e.EndTime, &e.AllDay, &e.IsShared, &e.SyncedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// ListUpcomingCalendarEventsForReminder returns events starting within the next N minutes
func (s *Storage) ListUpcomingCalendarEventsForReminder(minutes int) ([]*domain.CalendarEvent, error) {
	now := time.Now()
	threshold := now.Add(time.Duration(minutes) * time.Minute)

	rows, err := s.db.Query(
		`SELECT id, user_id, caldav_uid, title, description, location, start_time, end_time, all_day, is_shared, synced_at, created_at, updated_at
		 FROM calendar_events
		 WHERE start_time > ? AND start_time <= ? AND all_day = 0
		 ORDER BY start_time ASC`,
		now, threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.CalendarEvent
	for rows.Next() {
		e := &domain.CalendarEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.CalDAVUID, &e.Title, &e.Description, &e.Location, &e.StartTime, &e.EndTime, &e.AllDay, &e.IsShared, &e.SyncedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}
