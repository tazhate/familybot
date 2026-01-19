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

// === Tasks ===

func (s *Storage) CreateTask(t *domain.Task) error {
	res, err := s.db.Exec(
		`INSERT INTO tasks (user_id, assigned_to, title, description, priority, is_shared, due_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.AssignedTo, t.Title, t.Description, t.Priority, t.IsShared, t.DueDate,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	t.CreatedAt = time.Now()
	return nil
}

func (s *Storage) GetTask(id int64) (*domain.Task, error) {
	t := &domain.Task{}
	err := s.db.QueryRow(
		`SELECT id, user_id, assigned_to, title, description, priority, is_shared, due_date, done_at, created_at
		 FROM tasks WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.UserID, &t.AssignedTo, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Storage) ListTasksByUser(userID int64, includeShared bool, includeDone bool) ([]*domain.Task, error) {
	query := `SELECT id, user_id, assigned_to, title, description, priority, is_shared, due_date, done_at, created_at
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
		if err := rows.Scan(&t.ID, &t.UserID, &t.AssignedTo, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Storage) ListTasksForToday(userID int64) ([]*domain.Task, error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	rows, err := s.db.Query(
		`SELECT id, user_id, assigned_to, title, description, priority, is_shared, due_date, done_at, created_at
		 FROM tasks
		 WHERE (user_id = ? OR assigned_to = ? OR is_shared = 1)
		   AND done_at IS NULL
		   AND (priority = 'urgent' OR (due_date >= ? AND due_date < ?))
		 ORDER BY
		   CASE priority WHEN 'urgent' THEN 1 WHEN 'week' THEN 2 ELSE 3 END,
		   due_date ASC`,
		userID, userID, today, tomorrow,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t := &domain.Task{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.AssignedTo, &t.Title, &t.Description, &t.Priority, &t.IsShared, &t.DueDate, &t.DoneAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Storage) MarkTaskDone(id int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET done_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

func (s *Storage) DeleteTask(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
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

// === Persons ===

func (s *Storage) CreatePerson(p *domain.Person) error {
	res, err := s.db.Exec(
		`INSERT INTO persons (user_id, name, role, birthday, notes) VALUES (?, ?, ?, ?, ?)`,
		p.UserID, p.Name, p.Role, p.Birthday, p.Notes,
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
		`SELECT id, user_id, name, role, birthday, notes, created_at FROM persons WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Storage) GetPersonByName(userID int64, name string) (*domain.Person, error) {
	p := &domain.Person{}
	err := s.db.QueryRow(
		`SELECT id, user_id, name, role, birthday, notes, created_at FROM persons WHERE user_id = ? AND LOWER(name) = LOWER(?)`,
		userID, name,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Storage) ListPersonsByUser(userID int64) ([]*domain.Person, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, role, birthday, notes, created_at
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, nil
}

func (s *Storage) ListPersonsWithBirthday(userID int64) ([]*domain.Person, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, role, birthday, notes, created_at
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, nil
}

func (s *Storage) ListUpcomingBirthdays(userID int64, days int) ([]*domain.Person, error) {
	// Get persons whose birthday is within the next N days
	rows, err := s.db.Query(
		`SELECT id, user_id, name, role, birthday, notes, created_at
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Role, &p.Birthday, &p.Notes, &p.CreatedAt); err != nil {
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
		`UPDATE persons SET name = ?, role = ?, birthday = ?, notes = ? WHERE id = ?`,
		p.Name, p.Role, p.Birthday, p.Notes, p.ID,
	)
	return err
}

func (s *Storage) DeletePerson(id int64) error {
	_, err := s.db.Exec(`DELETE FROM persons WHERE id = ?`, id)
	return err
}
