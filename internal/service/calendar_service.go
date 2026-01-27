package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/tazhate/familybot/internal/clients/caldav"
	"github.com/tazhate/familybot/internal/domain"
	"github.com/tazhate/familybot/internal/storage"
)

// CalendarService handles calendar operations and syncing with Apple Calendar
type CalendarService struct {
	storage      *storage.Storage
	caldavClient *caldav.Client
	calendarPath string         // Path to the calendar to sync
	ownerUserID  int64          // Owner user ID for new events
	timezone     *time.Location // Timezone for event times
}

// NewCalendarService creates a new calendar service
func NewCalendarService(s *storage.Storage, client *caldav.Client, ownerUserID int64, tz *time.Location) *CalendarService {
	if tz == nil {
		tz = time.UTC
	}
	return &CalendarService{
		storage:      s,
		caldavClient: client,
		ownerUserID:  ownerUserID,
		timezone:     tz,
	}
}

// IsConfigured returns true if CalDAV client is configured
func (s *CalendarService) IsConfigured() bool {
	return s.caldavClient != nil && s.caldavClient.IsConfigured()
}

// SetCalendarPath sets the calendar path to use for sync
func (s *CalendarService) SetCalendarPath(path string) {
	s.calendarPath = path
	if s.caldavClient != nil {
		s.caldavClient.SetCalendarID(path)
	}
}

// DiscoverCalendars returns available calendars from Apple
func (s *CalendarService) DiscoverCalendars() ([]caldav.Calendar, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("CalDAV not configured")
	}
	return s.caldavClient.DiscoverCalendars()
}

// SyncResult contains sync operation results
type SyncResult struct {
	Added   int
	Updated int
	Deleted int
	Errors  []string
}

// SyncFromApple syncs events from Apple Calendar to local storage
func (s *CalendarService) SyncFromApple() (*SyncResult, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("CalDAV not configured")
	}

	if s.calendarPath == "" {
		return nil, fmt.Errorf("calendar path not set")
	}

	result := &SyncResult{}

	// Get events for next 90 days
	from := time.Now().Truncate(24 * time.Hour)
	to := from.AddDate(0, 3, 0) // 3 months ahead

	appleEvents, err := s.caldavClient.GetEvents(s.calendarPath, from, to)
	if err != nil {
		return nil, fmt.Errorf("get events from Apple: %w", err)
	}

	// Get existing local events
	localEvents, err := s.storage.ListAllCalendarEvents()
	if err != nil {
		return nil, fmt.Errorf("get local events: %w", err)
	}

	// Create a map of local events by CalDAV UID
	localByUID := make(map[string]*domain.CalendarEvent)
	for _, e := range localEvents {
		if e.CalDAVUID != "" {
			localByUID[e.CalDAVUID] = e
		}
	}

	// Track which UIDs we've seen from Apple
	seenUIDs := make(map[string]bool)
	now := time.Now()

	// Process Apple events
	for _, ae := range appleEvents {
		seenUIDs[ae.UID] = true

		local, exists := localByUID[ae.UID]
		if exists {
			// Update existing event if changed
			if s.eventChanged(local, &ae) {
				local.Title = ae.Summary
				local.Description = ae.Description
				local.Location = ae.Location
				local.StartTime = ae.StartTime
				local.EndTime = ae.EndTime
				local.AllDay = ae.AllDay
				local.SyncedAt = &now

				if err := s.storage.UpdateCalendarEvent(local); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update %s: %v", ae.UID, err))
				} else {
					result.Updated++
				}
			}
		} else {
			// Create new local event
			event := &domain.CalendarEvent{
				UserID:      s.ownerUserID,
				CalDAVUID:   ae.UID,
				Title:       ae.Summary,
				Description: ae.Description,
				Location:    ae.Location,
				StartTime:   ae.StartTime,
				EndTime:     ae.EndTime,
				AllDay:      ae.AllDay,
				IsShared:    false, // Don't auto-share calendar events to avoid duplication
				SyncedAt:    &now,
			}

			if err := s.storage.CreateCalendarEvent(event); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create %s: %v", ae.UID, err))
			} else {
				result.Added++
			}
		}
	}

	// Delete local events that no longer exist in Apple
	for uid, local := range localByUID {
		if !seenUIDs[uid] && local.SyncedAt != nil {
			// Only delete if it was synced from Apple (has SyncedAt)
			if err := s.storage.DeleteCalendarEvent(local.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", uid, err))
			} else {
				result.Deleted++
			}
		}
	}

	return result, nil
}

// eventChanged checks if Apple event differs from local
func (s *CalendarService) eventChanged(local *domain.CalendarEvent, apple *caldav.Event) bool {
	if local.Title != apple.Summary {
		return true
	}
	if local.Description != apple.Description {
		return true
	}
	if local.Location != apple.Location {
		return true
	}
	if !local.StartTime.Equal(apple.StartTime) {
		return true
	}
	if !local.EndTime.Equal(apple.EndTime) {
		return true
	}
	if local.AllDay != apple.AllDay {
		return true
	}
	return false
}

// CreateEvent creates a new event locally and syncs to Apple
func (s *CalendarService) CreateEvent(userID int64, title string, startTime time.Time, endTime time.Time, location string, allDay bool) (*domain.CalendarEvent, error) {
	event := &domain.CalendarEvent{
		UserID:      userID,
		Title:       title,
		StartTime:   startTime,
		EndTime:     endTime,
		Location:    location,
		AllDay:      allDay,
		IsShared:    true,
	}

	// Create locally first
	if err := s.storage.CreateCalendarEvent(event); err != nil {
		return nil, fmt.Errorf("create local event: %w", err)
	}

	// Sync to Apple if configured
	if s.IsConfigured() && s.calendarPath != "" {
		appleEvent := &caldav.Event{
			Summary:     title,
			Description: "",
			Location:    location,
			StartTime:   startTime,
			EndTime:     endTime,
			AllDay:      allDay,
		}

		if err := s.caldavClient.CreateEvent(s.calendarPath, appleEvent); err != nil {
			// Log error but don't fail - local event is created
			fmt.Printf("Warning: failed to sync event to Apple: %v\n", err)
		} else {
			// Update local event with CalDAV UID
			event.CalDAVUID = appleEvent.UID
			now := time.Now()
			event.SyncedAt = &now
			_ = s.storage.UpdateCalendarEvent(event)
		}
	}

	return event, nil
}

// UpdateEvent updates an event locally and syncs to Apple
func (s *CalendarService) UpdateEvent(event *domain.CalendarEvent) error {
	// Update locally first
	if err := s.storage.UpdateCalendarEvent(event); err != nil {
		return fmt.Errorf("update local event: %w", err)
	}

	// Sync to Apple if configured and event has CalDAV UID
	if s.IsConfigured() && s.calendarPath != "" && event.CalDAVUID != "" {
		appleEvent := &caldav.Event{
			UID:         event.CalDAVUID,
			Summary:     event.Title,
			Description: event.Description,
			Location:    event.Location,
			StartTime:   event.StartTime,
			EndTime:     event.EndTime,
			AllDay:      event.AllDay,
		}

		if err := s.caldavClient.UpdateEvent(s.calendarPath, appleEvent); err != nil {
			// Log error but don't fail - local event is updated
			fmt.Printf("Warning: failed to sync event update to Apple: %v\n", err)
		} else {
			// Update sync time
			now := time.Now()
			event.SyncedAt = &now
			_ = s.storage.UpdateCalendarEvent(event)
		}
	}

	return nil
}

// DeleteEvent deletes an event locally and from Apple
func (s *CalendarService) DeleteEvent(eventID int64, userID int64) error {
	event, err := s.storage.GetCalendarEvent(eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}
	if event == nil {
		return fmt.Errorf("event not found")
	}

	// Delete from Apple if synced
	if s.IsConfigured() && event.CalDAVUID != "" && s.calendarPath != "" {
		if err := s.caldavClient.DeleteEvent(s.calendarPath, event.CalDAVUID); err != nil {
			// Log but continue with local delete
			fmt.Printf("Warning: failed to delete from Apple: %v\n", err)
		}
	}

	return s.storage.DeleteCalendarEvent(eventID)
}

// ListToday returns today's events
func (s *CalendarService) ListToday(userID int64) ([]*domain.CalendarEvent, error) {
	return s.storage.ListCalendarEventsToday(userID, true)
}

// ListWeek returns this week's events
func (s *CalendarService) ListWeek(userID int64) ([]*domain.CalendarEvent, error) {
	return s.storage.ListCalendarEventsWeek(userID, true)
}

// ListRange returns events in a date range
func (s *CalendarService) ListRange(userID int64, from, to time.Time) ([]*domain.CalendarEvent, error) {
	return s.storage.ListCalendarEvents(userID, from, to, true)
}

// GetUpcomingForReminder returns events starting within the next N minutes
func (s *CalendarService) GetUpcomingForReminder(minutes int) ([]*domain.CalendarEvent, error) {
	return s.storage.ListUpcomingCalendarEventsForReminder(minutes)
}

// FormatEventList formats events for display
func (s *CalendarService) FormatEventList(events []*domain.CalendarEvent) string {
	if len(events) == 0 {
		return "Нет событий"
	}

	var sb strings.Builder
	var currentDate string

	for _, e := range events {
		eventDate := e.StartTime.Format("02.01")

		// Add date header if changed
		if eventDate != currentDate {
			if currentDate != "" {
				sb.WriteString("\n")
			}
			dayName := russianWeekday(e.StartTime.Weekday())
			sb.WriteString(fmt.Sprintf("📅 %s, %s:\n", eventDate, dayName))
			currentDate = eventDate
		}

		// Format event line
		var line string
		if e.AllDay {
			line = fmt.Sprintf("  🗓 %s", e.Title)
		} else {
			line = fmt.Sprintf("  %s — %s", e.FormatTime(), e.Title)
		}

		if e.Location != "" {
			line += fmt.Sprintf(" 📍%s", e.Location)
		}

		sb.WriteString(line + "\n")
	}

	return sb.String()
}

// FormatTodayBriefing formats today's events for morning briefing
func (s *CalendarService) FormatTodayBriefing(events []*domain.CalendarEvent) string {
	if len(events) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("📅 События сегодня:\n")

	for _, e := range events {
		var line string
		if e.AllDay {
			line = fmt.Sprintf("• %s (весь день)", e.Title)
		} else {
			line = fmt.Sprintf("• %s — %s", e.StartTime.Format("15:04"), e.Title)
		}

		if e.Location != "" {
			line += fmt.Sprintf(" 📍%s", e.Location)
		}

		sb.WriteString(line + "\n")
	}

	return sb.String()
}

// TaskToEvent converts a task with due date to a calendar event
func (s *CalendarService) TaskToEvent(task *domain.Task) *domain.CalendarEvent {
	if task.DueDate == nil {
		return nil
	}

	return &domain.CalendarEvent{
		UserID:      task.UserID,
		Title:       "📋 " + task.Title,
		Description: fmt.Sprintf("Задача #%d", task.ID),
		StartTime:   *task.DueDate,
		EndTime:     task.DueDate.Add(time.Hour),
		AllDay:      true,
		IsShared:    task.IsShared,
	}
}

// russianWeekday returns Russian weekday name
func russianWeekday(wd time.Weekday) string {
	days := []string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	return days[wd]
}

// SyncTaskToCalendar creates or updates a calendar event for a task with due_date
func (s *CalendarService) SyncTaskToCalendar(task *domain.Task) error {
	if task == nil || task.DueDate == nil {
		return nil // Nothing to sync
	}

	if !s.IsConfigured() || s.calendarPath == "" {
		return nil // CalDAV not configured
	}

	// Create calendar event from task
	appleEvent := &caldav.Event{
		UID:         fmt.Sprintf("task-%d@familybot", task.ID),
		Summary:     "📋 " + task.Title,
		Description: fmt.Sprintf("Задача #%d из FamilyBot", task.ID),
		StartTime:   *task.DueDate,
		EndTime:     task.DueDate.Add(time.Hour),
		AllDay:      true,
	}

	if err := s.caldavClient.CreateEvent(s.calendarPath, appleEvent); err != nil {
		return fmt.Errorf("sync task to Apple Calendar: %w", err)
	}

	return nil
}

// DeleteTaskFromCalendar removes calendar event for a completed/deleted task
func (s *CalendarService) DeleteTaskFromCalendar(taskID int64) error {
	if !s.IsConfigured() || s.calendarPath == "" {
		return nil // CalDAV not configured
	}

	// Use the same UID format as SyncTaskToCalendar
	uid := fmt.Sprintf("task-%d@familybot", taskID)

	if err := s.caldavClient.DeleteEvent(s.calendarPath, uid); err != nil {
		// Don't fail if event doesn't exist
		if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("delete task from Apple Calendar: %w", err)
		}
	}

	return nil
}

// weekdayToRRULE converts Go weekday to RRULE BYDAY format
func weekdayToRRULE(wd int) string {
	days := []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
	if wd < 0 || wd > 6 {
		return "MO"
	}
	return days[wd]
}

// SyncWeeklyEventToCalendar creates or updates a recurring event in Apple Calendar
func (s *CalendarService) SyncWeeklyEventToCalendar(eventID int64, dayOfWeek int, timeStart, timeEnd, title string, isFloating bool, floatingDays []int) error {
	if !s.IsConfigured() || s.calendarPath == "" {
		return nil // CalDAV not configured
	}

	// Use configured timezone
	tz := s.timezone
	if tz == nil {
		tz = time.UTC
	}

	// Parse start time
	now := time.Now().In(tz)
	startHour, startMin := 0, 0
	if timeStart != "" {
		fmt.Sscanf(timeStart, "%d:%d", &startHour, &startMin)
	}

	// Calculate next occurrence of this day
	daysUntil := (dayOfWeek - int(now.Weekday()) + 7) % 7
	if daysUntil == 0 && (now.Hour() > startHour || (now.Hour() == startHour && now.Minute() >= startMin)) {
		daysUntil = 7 // Already passed today, next week
	}
	startTime := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, startHour, startMin, 0, 0, tz)

	// Calculate end time
	var endTime time.Time
	if timeEnd != "" {
		endHour, endMin := 0, 0
		fmt.Sscanf(timeEnd, "%d:%d", &endHour, &endMin)
		endTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), endHour, endMin, 0, 0, tz)
	} else {
		endTime = startTime.Add(time.Hour) // Default 1 hour
	}

	// Build recurrence rule
	var rrule string
	if isFloating && len(floatingDays) > 0 {
		// Floating event: multiple possible days
		var byDays []string
		for _, d := range floatingDays {
			byDays = append(byDays, weekdayToRRULE(d))
		}
		rrule = fmt.Sprintf("FREQ=WEEKLY;BYDAY=%s", strings.Join(byDays, ","))
	} else {
		// Regular weekly event
		rrule = fmt.Sprintf("FREQ=WEEKLY;BYDAY=%s", weekdayToRRULE(dayOfWeek))
	}

	// Create event
	appleEvent := &caldav.Event{
		UID:         fmt.Sprintf("schedule-%d@familybot", eventID),
		Summary:     "🗓 " + title,
		Description: fmt.Sprintf("Недельное расписание #%d из FamilyBot", eventID),
		StartTime:   startTime,
		EndTime:     endTime,
		AllDay:      timeStart == "", // All-day if no specific time
		RRule:       rrule,
	}

	if err := s.caldavClient.CreateEvent(s.calendarPath, appleEvent); err != nil {
		return fmt.Errorf("sync weekly event to Apple Calendar: %w", err)
	}

	return nil
}

// DeleteWeeklyEventFromCalendar removes a recurring event from Apple Calendar
func (s *CalendarService) DeleteWeeklyEventFromCalendar(eventID int64) error {
	if !s.IsConfigured() || s.calendarPath == "" {
		return nil // CalDAV not configured
	}

	uid := fmt.Sprintf("schedule-%d@familybot", eventID)

	if err := s.caldavClient.DeleteEvent(s.calendarPath, uid); err != nil {
		// Don't fail if event doesn't exist
		if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("delete weekly event from Apple Calendar: %w", err)
		}
	}

	return nil
}
