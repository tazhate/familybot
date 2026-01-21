package domain

import (
	"encoding/json"
	"time"
)

// ChecklistItem represents a single item in a checklist
type ChecklistItem struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// Checklist represents a reusable checklist template
type Checklist struct {
	ID        int64
	UserID    int64
	Title     string // e.g. "Тим", "Перед поездкой"
	Items     []ChecklistItem
	PersonID  *int64 // Optional link to a Person
	CreatedAt time.Time
}

// ItemsJSON returns items as JSON string for storage
func (c *Checklist) ItemsJSON() string {
	data, _ := json.Marshal(c.Items)
	return string(data)
}

// ParseItemsJSON parses items from JSON string
func (c *Checklist) ParseItemsJSON(data string) error {
	if data == "" {
		c.Items = []ChecklistItem{}
		return nil
	}
	return json.Unmarshal([]byte(data), &c.Items)
}

// ResetChecks resets all items to unchecked
func (c *Checklist) ResetChecks() {
	for i := range c.Items {
		c.Items[i].Checked = false
	}
}

// CheckItem marks an item as checked by index
func (c *Checklist) CheckItem(index int) bool {
	if index < 0 || index >= len(c.Items) {
		return false
	}
	c.Items[index].Checked = true
	return true
}

// AllChecked returns true if all items are checked
func (c *Checklist) AllChecked() bool {
	for _, item := range c.Items {
		if !item.Checked {
			return false
		}
	}
	return len(c.Items) > 0
}

// CheckedCount returns number of checked items
func (c *Checklist) CheckedCount() int {
	count := 0
	for _, item := range c.Items {
		if item.Checked {
			count++
		}
	}
	return count
}
