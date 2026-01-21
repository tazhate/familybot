package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type AutoService struct {
	storage *storage.Storage
}

func NewAutoService(s *storage.Storage) *AutoService {
	return &AutoService{storage: s}
}

func (s *AutoService) Create(userID int64, name string, year int) (*domain.Auto, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("название машины не может быть пустым")
	}

	auto := &domain.Auto{
		UserID: userID,
		Name:   name,
		Year:   year,
	}

	if err := s.storage.CreateAuto(auto); err != nil {
		return nil, fmt.Errorf("create auto: %w", err)
	}

	return auto, nil
}

func (s *AutoService) List(userID int64) ([]*domain.Auto, error) {
	return s.storage.ListAutosByUser(userID)
}

func (s *AutoService) Get(id int64) (*domain.Auto, error) {
	return s.storage.GetAuto(id)
}

func (s *AutoService) Delete(id int64, userID int64) error {
	auto, err := s.storage.GetAuto(id)
	if err != nil {
		return fmt.Errorf("get auto: %w", err)
	}
	if auto == nil {
		return fmt.Errorf("машина не найдена")
	}
	if auto.UserID != userID {
		return fmt.Errorf("нет доступа")
	}
	return s.storage.DeleteAuto(id)
}

func (s *AutoService) SetInsurance(id int64, userID int64, until time.Time) error {
	auto, err := s.storage.GetAuto(id)
	if err != nil {
		return fmt.Errorf("get auto: %w", err)
	}
	if auto == nil {
		return fmt.Errorf("машина не найдена")
	}
	if auto.UserID != userID {
		return fmt.Errorf("нет доступа")
	}
	return s.storage.UpdateAutoInsurance(id, until)
}

func (s *AutoService) SetMaintenance(id int64, userID int64, until time.Time) error {
	auto, err := s.storage.GetAuto(id)
	if err != nil {
		return fmt.Errorf("get auto: %w", err)
	}
	if auto == nil {
		return fmt.Errorf("машина не найдена")
	}
	if auto.UserID != userID {
		return fmt.Errorf("нет доступа")
	}
	return s.storage.UpdateAutoMaintenance(id, until)
}

// ListNeedingReminder returns autos needing reminder within N days
func (s *AutoService) ListNeedingReminder(days int) ([]*domain.Auto, error) {
	return s.storage.ListAutosNeedingReminder(days)
}

// ParseAddArgs parses "Название ГГГГ" or just "Название"
func (s *AutoService) ParseAddArgs(args string) (name string, year int, err error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", 0, fmt.Errorf("укажи название машины")
	}

	// Try to find year at the end
	re := regexp.MustCompile(`\s+(\d{4})$`)
	if match := re.FindStringSubmatch(args); match != nil {
		year, _ = strconv.Atoi(match[1])
		name = strings.TrimSpace(re.ReplaceAllString(args, ""))
	} else {
		name = args
	}

	return name, year, nil
}

// ParseDate parses "ДД.ММ.ГГГГ" or "ДД.ММ"
func (s *AutoService) ParseDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	// Try full date first
	if t, err := time.Parse("02.01.2006", dateStr); err == nil {
		return t, nil
	}

	// Try short date (assume current year)
	if t, err := time.Parse("02.01", dateStr); err == nil {
		t = time.Date(time.Now().Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		// If date is in the past, use next year
		if t.Before(time.Now()) {
			t = t.AddDate(1, 0, 0)
		}
		return t, nil
	}

	return time.Time{}, fmt.Errorf("неверный формат даты, используй ДД.ММ.ГГГГ")
}

// FormatAutoList formats autos list for display
func (s *AutoService) FormatAutoList(autos []*domain.Auto) string {
	if len(autos) == 0 {
		return "Нет машин"
	}

	var sb strings.Builder
	for _, a := range autos {
		yearStr := ""
		if a.Year > 0 {
			yearStr = fmt.Sprintf(" (%d)", a.Year)
		}
		sb.WriteString(fmt.Sprintf("🚗 <b>#%d</b> %s%s\n", a.ID, a.Name, yearStr))

		if a.InsuranceUntil != nil {
			days := a.DaysUntilInsurance()
			sb.WriteString(fmt.Sprintf("   📋 Страховка: %s (%s, %d дн.)\n",
				a.InsuranceUntil.Format("02.01.2006"), a.InsuranceStatus(), days))
		}
		if a.MaintenanceUntil != nil {
			days := a.DaysUntilMaintenance()
			sb.WriteString(fmt.Sprintf("   🔧 ТО: %s (%s, %d дн.)\n",
				a.MaintenanceUntil.Format("02.01.2006"), a.MaintenanceStatus(), days))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
