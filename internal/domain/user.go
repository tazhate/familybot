package domain

import "time"

type UserRole string

const (
	RoleOwner   UserRole = "owner"
	RolePartner UserRole = "partner"
)

type User struct {
	ID         int64
	TelegramID int64
	Name       string
	Role       UserRole
	CreatedAt  time.Time
}
