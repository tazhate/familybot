package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type PersonService struct {
	storage         *storage.Storage
	reminderService *ReminderService
}

func NewPersonService(s *storage.Storage) *PersonService {
	return &PersonService{storage: s}
}

// SetReminderService sets the reminder service for auto-creating birthday reminders
func (s *PersonService) SetReminderService(rs *ReminderService) {
	s.reminderService = rs
}

// Create creates a new person and auto-creates birthday reminders if birthday is provided
func (s *PersonService) Create(userID int64, name string, role domain.PersonRole, birthday *time.Time, notes string) (*domain.Person, error) {
	if name == "" {
		return nil, errors.New("имя не может быть пустым")
	}

	// Check if person with this name already exists
	existing, _ := s.storage.GetPersonByName(userID, name)
	if existing != nil {
		return nil, errors.New("человек с таким именем уже существует")
	}

	person := &domain.Person{
		UserID:   userID,
		Name:     name,
		Role:     role,
		Birthday: birthday,
		Notes:    notes,
	}

	if err := s.storage.CreatePerson(person); err != nil {
		return nil, err
	}

	// Auto-create birthday reminders if birthday is provided
	if birthday != nil && s.reminderService != nil {
		s.createBirthdayReminders(userID, person)
	}

	return person, nil
}

// createBirthdayReminders creates yearly birthday reminders (7 days before, 1 day before, on the day)
func (s *PersonService) createBirthdayReminders(userID int64, person *domain.Person) {
	if person.Birthday == nil {
		return
	}

	// Reminder configurations: days before, time, title format
	reminders := []struct {
		daysBefore int
		time       string
		titleFmt   string
	}{
		{7, "11:00", "🎂 Через неделю ДР: %s"},
		{1, "11:00", "🎂 Завтра ДР: %s"},
		{0, "11:00", "🎉 Сегодня ДР: %s!"},
	}

	for _, r := range reminders {
		// Calculate the reminder date
		bdMonth := int(person.Birthday.Month())
		bdDay := person.Birthday.Day() - r.daysBefore

		// Handle day overflow (e.g., if birthday is on 1st and we need 7 days before)
		reminderMonth := bdMonth
		if bdDay <= 0 {
			reminderMonth--
			if reminderMonth <= 0 {
				reminderMonth = 12
			}
			// Get days in previous month
			prevMonthDays := daysInMonth(reminderMonth)
			bdDay = prevMonthDays + bdDay
		}

		params := domain.ReminderParams{
			Time:  r.time,
			Month: reminderMonth,
			Day:   bdDay,
		}

		title := fmt.Sprintf(r.titleFmt, person.Name)
		if person.Birthday.Year() > 1 {
			// Calculate age they will turn
			age := person.Age()
			if r.daysBefore > 0 {
				age++ // They haven't had their birthday yet
			}
			title = fmt.Sprintf(r.titleFmt+" (%d лет)", person.Name, age)
		}

		_, err := s.reminderService.Create(userID, title, domain.ReminderYearly, params)
		if err != nil {
			// Log but don't fail - person was already created
			fmt.Printf("Warning: failed to create birthday reminder: %v\n", err)
		}
	}
}

// daysInMonth returns the number of days in a given month (non-leap year)
func daysInMonth(month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		return 28 // simplified, doesn't handle leap years
	default:
		return 30
	}
}

// Get returns a person by ID
func (s *PersonService) Get(id int64) (*domain.Person, error) {
	return s.storage.GetPerson(id)
}

// GetByName returns a person by name (case-insensitive)
func (s *PersonService) GetByName(userID int64, name string) (*domain.Person, error) {
	return s.storage.GetPersonByName(userID, name)
}

// List returns all persons for a user
func (s *PersonService) List(userID int64) ([]*domain.Person, error) {
	return s.storage.ListPersonsByUser(userID)
}

// GetNamesMap returns a map of person ID to name for a user
func (s *PersonService) GetNamesMap(userID int64) (map[int64]string, error) {
	persons, err := s.storage.ListPersonsByUser(userID)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]string)
	for _, p := range persons {
		result[p.ID] = p.Name
	}
	return result, nil
}

// ListBirthdays returns persons with birthdays
func (s *PersonService) ListBirthdays(userID int64) ([]*domain.Person, error) {
	return s.storage.ListPersonsWithBirthday(userID)
}

// ListUpcomingBirthdays returns persons with birthdays in the next N days
func (s *PersonService) ListUpcomingBirthdays(userID int64, days int) ([]*domain.Person, error) {
	return s.storage.ListUpcomingBirthdays(userID, days)
}

// Update updates a person
func (s *PersonService) Update(person *domain.Person) error {
	return s.storage.UpdatePerson(person)
}

// LinkToTelegram links a person to a Telegram user
func (s *PersonService) LinkToTelegram(personID int64, telegramID int64) error {
	return s.storage.UpdatePersonTelegramID(personID, &telegramID)
}

// UnlinkFromTelegram removes Telegram link from a person
func (s *PersonService) UnlinkFromTelegram(personID int64) error {
	return s.storage.UpdatePersonTelegramID(personID, nil)
}

// Delete deletes a person
func (s *PersonService) Delete(id int64, userID int64) error {
	person, err := s.storage.GetPerson(id)
	if err != nil {
		return err
	}
	if person == nil {
		return errors.New("человек не найден")
	}
	if person.UserID != userID {
		return errors.New("нет доступа")
	}
	return s.storage.DeletePerson(id)
}

// ParseAddPersonArgs parses "/addperson Тим ребёнок 12.06.2017" format
func (s *PersonService) ParseAddPersonArgs(args string) (name string, role domain.PersonRole, birthday *time.Time, err error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		err = errors.New("укажите имя")
		return
	}

	name = parts[0]
	role = domain.RoleContact // default

	if len(parts) >= 2 {
		// Try to parse role
		switch strings.ToLower(parts[1]) {
		case "ребёнок", "ребенок", "child":
			role = domain.RoleChild
		case "семья", "family":
			role = domain.RoleFamily
		case "контакт", "contact":
			role = domain.RoleContact
		case "партнёр_ребёнок", "partner_child":
			role = domain.RolePartnerChild
		default:
			// Maybe it's a date?
			if bd := s.parseDate(parts[1]); bd != nil {
				birthday = bd
			}
		}
	}

	if len(parts) >= 3 && birthday == nil {
		// Third part should be birthday
		birthday = s.parseDate(parts[2])
	}

	return
}

// parseDate parses date in formats: DD.MM.YYYY, DD.MM, DD/MM/YYYY, DD/MM
func (s *PersonService) parseDate(str string) *time.Time {
	formats := []string{
		"02.01.2006", // DD.MM.YYYY
		"02.01",      // DD.MM (without year)
		"02/01/2006", // DD/MM/YYYY
		"02/01",      // DD/MM
		"2.1.2006",   // D.M.YYYY
		"2.1",        // D.M
	}

	// Check if it matches date pattern
	datePattern := regexp.MustCompile(`^\d{1,2}[./]\d{1,2}([./]\d{4})?$`)
	if !datePattern.MatchString(str) {
		return nil
	}

	// Normalize separators
	normalized := strings.ReplaceAll(str, "/", ".")

	for _, format := range formats {
		normalizedFormat := strings.ReplaceAll(format, "/", ".")
		if t, err := time.Parse(normalizedFormat, normalized); err == nil {
			// If year is 0 (not specified), set it to 0001 to indicate "year unknown"
			if !strings.Contains(str, "2") && !strings.Contains(str, "1") {
				// No year in string - use year 1 as marker for "no year"
				t = time.Date(1, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			}
			return &t
		}
	}

	return nil
}

// FormatPersonList formats persons list for display
func (s *PersonService) FormatPersonList(persons []*domain.Person) string {
	if len(persons) == 0 {
		return "Список пуст"
	}

	var sb strings.Builder
	for _, p := range persons {
		sb.WriteString(fmt.Sprintf("%s <b>%s</b>", p.RoleEmoji(), p.Name))
		if p.HasBirthday() {
			if p.Birthday.Year() > 1 {
				sb.WriteString(fmt.Sprintf(" (%d лет)", p.Age()))
			}
			sb.WriteString(fmt.Sprintf(" 🎂 %s", p.Birthday.Format("02.01")))
			days := p.DaysUntilBirthday()
			if days == 0 {
				sb.WriteString(" <b>СЕГОДНЯ!</b>")
			} else if days <= 7 {
				sb.WriteString(fmt.Sprintf(" (через %d дн.)", days))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatBirthdaysList formats upcoming birthdays
func (s *PersonService) FormatBirthdaysList(persons []*domain.Person) string {
	if len(persons) == 0 {
		return "Ближайших дней рождения нет"
	}

	var sb strings.Builder
	for _, p := range persons {
		days := p.DaysUntilBirthday()
		sb.WriteString(fmt.Sprintf("%s <b>%s</b>", p.RoleEmoji(), p.Name))

		if p.Birthday.Year() > 1 {
			nextAge := p.Age()
			if days > 0 {
				nextAge++
			}
			sb.WriteString(fmt.Sprintf(" — %d лет", nextAge))
		}

		sb.WriteString(fmt.Sprintf("\n   🎂 %s", p.Birthday.Format("02 января")))

		if days == 0 {
			sb.WriteString(" — <b>СЕГОДНЯ!</b>")
		} else if days == 1 {
			sb.WriteString(" — завтра")
		} else {
			sb.WriteString(fmt.Sprintf(" — через %d дн.", days))
		}
		sb.WriteString("\n\n")
	}
	return sb.String()
}
