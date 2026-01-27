package caldav

import "time"

// Calendar represents an iCloud calendar
type Calendar struct {
	ID          string // Calendar path/URL
	DisplayName string
	Color       string
	URL         string
}

// Event represents a calendar event
type Event struct {
	UID         string // Unique ID in CalDAV
	Summary     string // Title
	Description string
	Location    string
	StartTime   time.Time
	EndTime     time.Time
	AllDay      bool
	Reminders   []Reminder
	RRule       string // Recurrence rule (e.g., "FREQ=WEEKLY;BYDAY=MO")
}

// Reminder represents an event reminder
type Reminder struct {
	MinutesBefore int
}
