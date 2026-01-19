package domain

import "time"

// PersonRole defines the type of person
type PersonRole string

const (
	RoleChild        PersonRole = "child"         // Дети (Тим, Лука)
	RoleFamily       PersonRole = "family"        // Семья (Ира)
	RoleContact      PersonRole = "contact"       // Контакты (Федя)
	RolePartnerChild PersonRole = "partner_child" // Дети партнёра
)

// Person represents a person in the family circle
type Person struct {
	ID        int64
	UserID    int64      // Owner user
	Name      string     // Display name
	Role      PersonRole // child, family, contact, partner_child
	Birthday  *time.Time // nil if unknown
	Notes     string     // Additional info
	CreatedAt time.Time
}

// Age returns current age if birthday is set
func (p *Person) Age() int {
	if p.Birthday == nil {
		return 0
	}
	now := time.Now()
	age := now.Year() - p.Birthday.Year()
	if now.YearDay() < p.Birthday.YearDay() {
		age--
	}
	return age
}

// HasBirthday returns true if birthday is set
func (p *Person) HasBirthday() bool {
	return p.Birthday != nil
}

// DaysUntilBirthday returns days until next birthday
func (p *Person) DaysUntilBirthday() int {
	if p.Birthday == nil {
		return -1
	}

	now := time.Now()
	thisYear := time.Date(now.Year(), p.Birthday.Month(), p.Birthday.Day(), 0, 0, 0, 0, now.Location())

	if thisYear.Before(now) {
		// Birthday already passed this year
		thisYear = thisYear.AddDate(1, 0, 0)
	}

	return int(thisYear.Sub(now).Hours() / 24)
}

// RoleEmoji returns emoji for the role
func (p *Person) RoleEmoji() string {
	switch p.Role {
	case RoleChild:
		return "👶"
	case RoleFamily:
		return "👨‍👩‍👧"
	case RoleContact:
		return "👤"
	case RolePartnerChild:
		return "👦"
	default:
		return "👤"
	}
}

// RoleName returns Russian name for the role
func (p *Person) RoleName() string {
	switch p.Role {
	case RoleChild:
		return "ребёнок"
	case RoleFamily:
		return "семья"
	case RoleContact:
		return "контакт"
	case RolePartnerChild:
		return "ребёнок партнёра"
	default:
		return "контакт"
	}
}
