package service

import (
	"fmt"
	"strings"

	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

type ChecklistService struct {
	storage *storage.Storage
}

func NewChecklistService(s *storage.Storage) *ChecklistService {
	return &ChecklistService{storage: s}
}

func (s *ChecklistService) Create(userID int64, title string, items []string) (*domain.Checklist, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("название не может быть пустым")
	}

	checklistItems := make([]domain.ChecklistItem, len(items))
	for i, item := range items {
		checklistItems[i] = domain.ChecklistItem{
			Text:    strings.TrimSpace(item),
			Checked: false,
		}
	}

	c := &domain.Checklist{
		UserID: userID,
		Title:  title,
		Items:  checklistItems,
	}

	if err := s.storage.CreateChecklist(c); err != nil {
		return nil, fmt.Errorf("create checklist: %w", err)
	}

	return c, nil
}

func (s *ChecklistService) Get(id int64) (*domain.Checklist, error) {
	return s.storage.GetChecklist(id)
}

func (s *ChecklistService) GetByTitle(userID int64, title string) (*domain.Checklist, error) {
	return s.storage.GetChecklistByTitle(userID, title)
}

func (s *ChecklistService) List(userID int64) ([]*domain.Checklist, error) {
	return s.storage.ListChecklistsByUser(userID)
}

func (s *ChecklistService) CheckItem(checklistID int64, userID int64, itemIndex int) error {
	c, err := s.storage.GetChecklist(checklistID)
	if err != nil {
		return fmt.Errorf("get checklist: %w", err)
	}
	if c == nil {
		return fmt.Errorf("чек-лист не найден")
	}
	if c.UserID != userID {
		return fmt.Errorf("нет доступа")
	}

	if !c.CheckItem(itemIndex) {
		return fmt.Errorf("неверный номер пункта")
	}

	return s.storage.UpdateChecklistItems(checklistID, c.Items)
}

func (s *ChecklistService) Reset(checklistID int64, userID int64) error {
	c, err := s.storage.GetChecklist(checklistID)
	if err != nil {
		return fmt.Errorf("get checklist: %w", err)
	}
	if c == nil {
		return fmt.Errorf("чек-лист не найден")
	}
	if c.UserID != userID {
		return fmt.Errorf("нет доступа")
	}

	c.ResetChecks()
	return s.storage.UpdateChecklistItems(checklistID, c.Items)
}

func (s *ChecklistService) Delete(checklistID int64, userID int64) error {
	c, err := s.storage.GetChecklist(checklistID)
	if err != nil {
		return fmt.Errorf("get checklist: %w", err)
	}
	if c == nil {
		return fmt.Errorf("чек-лист не найден")
	}
	if c.UserID != userID {
		return fmt.Errorf("нет доступа")
	}

	return s.storage.DeleteChecklist(checklistID)
}

func (s *ChecklistService) FormatChecklist(c *domain.Checklist) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 <b>%s</b>\n\n", c.Title))

	for i, item := range c.Items {
		status := "⬜"
		if item.Checked {
			status = "✅"
		}
		sb.WriteString(fmt.Sprintf("%s %d. %s\n", status, i+1, item.Text))
	}

	if c.AllChecked() {
		sb.WriteString("\n🎉 Все пункты выполнены!")
	} else {
		sb.WriteString(fmt.Sprintf("\n%d/%d выполнено", c.CheckedCount(), len(c.Items)))
	}

	return sb.String()
}

func (s *ChecklistService) FormatChecklistList(checklists []*domain.Checklist) string {
	if len(checklists) == 0 {
		return "Нет чек-листов"
	}

	var sb strings.Builder
	for _, c := range checklists {
		sb.WriteString(fmt.Sprintf("📋 <b>%s</b> (%d пунктов)\n", c.Title, len(c.Items)))
	}
	return sb.String()
}
