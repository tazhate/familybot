package caldav

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

const (
	// Apple iCloud CalDAV endpoint
	DefaultiCloudURL = "https://caldav.icloud.com"
)

// Client is a CalDAV client for Apple iCloud Calendar
type Client struct {
	baseURL    string
	username   string
	password   string
	calendarID string // Optional: specific calendar to use
	client     *caldav.Client
}

// NewClient creates a new CalDAV client
func NewClient(baseURL, username, password string) *Client {
	if baseURL == "" {
		baseURL = DefaultiCloudURL
	}
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
	}
}

// IsConfigured returns true if the client has credentials
func (c *Client) IsConfigured() bool {
	return c.username != "" && c.password != ""
}

// SetCalendarID sets the calendar to use
func (c *Client) SetCalendarID(id string) {
	c.calendarID = id
}

// connect establishes connection to CalDAV server
func (c *Client) connect() (*caldav.Client, error) {
	if c.client != nil {
		return c.client, nil
	}

	httpClient := &http.Client{
		Transport: &basicAuthTransport{
			username: c.username,
			password: c.password,
		},
		Timeout: 30 * time.Second,
	}

	client, err := caldav.NewClient(httpClient, c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to CalDAV: %w", err)
	}

	c.client = client
	return client, nil
}

// basicAuthTransport adds Basic Auth to HTTP requests
type basicAuthTransport struct {
	username string
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.username, t.password)
	return http.DefaultTransport.RoundTrip(req)
}

// DiscoverCalendars returns all calendars for the user
func (c *Client) DiscoverCalendars() ([]Calendar, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Find the user's calendar home
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("find principal: %w", err)
	}

	homeSet, err := client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("find home set: %w", err)
	}

	// Find all calendars
	cals, err := client.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("find calendars: %w", err)
	}

	var result []Calendar
	for _, cal := range cals {
		result = append(result, Calendar{
			ID:          cal.Path,
			DisplayName: cal.Name,
			URL:         cal.Path,
		})
	}

	return result, nil
}

// GetEvents returns events in the specified time range
func (c *Client) GetEvents(calendarPath string, from, to time.Time) ([]Event, error) {
	client, err := c.connect()
	if err != nil {
		return nil, err
	}

	if calendarPath == "" {
		calendarPath = c.calendarID
	}

	if calendarPath == "" {
		return nil, fmt.Errorf("calendar path not specified")
	}

	// Query events in date range
	query := &caldav.CalendarQuery{
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{
				{
					Name:  "VEVENT",
					Start: from,
					End:   to,
				},
			},
		},
	}

	objects, err := client.QueryCalendar(context.Background(), calendarPath, query)
	if err != nil {
		return nil, fmt.Errorf("query calendar: %w", err)
	}

	var events []Event
	for _, obj := range objects {
		event, err := parseCalendarObject(&obj)
		if err != nil {
			continue // Skip invalid events
		}
		events = append(events, event)
	}

	return events, nil
}

// CreateEvent creates a new event in the calendar
func (c *Client) CreateEvent(calendarPath string, event *Event) error {
	client, err := c.connect()
	if err != nil {
		return err
	}

	if calendarPath == "" {
		calendarPath = c.calendarID
	}

	if calendarPath == "" {
		return fmt.Errorf("calendar path not specified")
	}

	// Generate UID if not provided
	if event.UID == "" {
		event.UID = generateUID()
	}

	// Create iCalendar data
	cal := eventToICS(event)

	// Create path for new event
	eventPath := calendarPath
	if !strings.HasSuffix(eventPath, "/") {
		eventPath += "/"
	}
	eventPath += event.UID + ".ics"

	_, err = client.PutCalendarObject(context.Background(), eventPath, cal)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}

	return nil
}

// UpdateEvent updates an existing event
func (c *Client) UpdateEvent(calendarPath string, event *Event) error {
	// For CalDAV, update is the same as create (PUT replaces)
	return c.CreateEvent(calendarPath, event)
}

// DeleteEvent deletes an event by UID
func (c *Client) DeleteEvent(calendarPath, eventUID string) error {
	client, err := c.connect()
	if err != nil {
		return err
	}

	if calendarPath == "" {
		calendarPath = c.calendarID
	}

	eventPath := calendarPath
	if !strings.HasSuffix(eventPath, "/") {
		eventPath += "/"
	}
	eventPath += eventUID + ".ics"

	err = client.RemoveAll(context.Background(), eventPath)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}

	return nil
}

// parseCalendarObject parses a CalDAV object into an Event
func parseCalendarObject(obj *caldav.CalendarObject) (Event, error) {
	event := Event{}

	if obj.Data == nil {
		return event, fmt.Errorf("no data in calendar object")
	}

	cal := obj.Data

	// Find VEVENT component
	for _, comp := range cal.Children {
		if comp.Name != ical.CompEvent {
			continue
		}

		// Get UID
		if prop := comp.Props.Get(ical.PropUID); prop != nil {
			event.UID = prop.Value
		}

		// Get Summary (title)
		if prop := comp.Props.Get(ical.PropSummary); prop != nil {
			event.Summary = prop.Value
		}

		// Get Description
		if prop := comp.Props.Get(ical.PropDescription); prop != nil {
			event.Description = prop.Value
		}

		// Get Location
		if prop := comp.Props.Get(ical.PropLocation); prop != nil {
			event.Location = prop.Value
		}

		// Get start time
		if prop := comp.Props.Get(ical.PropDateTimeStart); prop != nil {
			t, err := prop.DateTime(time.UTC)
			if err == nil {
				event.StartTime = t
			}
			// Check if all-day event
			if valueType := prop.Params.Get(ical.ParamValue); valueType == string(ical.ValueDate) {
				event.AllDay = true
			}
		}

		// Get end time
		if prop := comp.Props.Get(ical.PropDateTimeEnd); prop != nil {
			t, err := prop.DateTime(time.UTC)
			if err == nil {
				event.EndTime = t
			}
		}

		break // Only process first VEVENT
	}

	return event, nil
}

// eventToICS converts an Event to iCalendar format
func eventToICS(event *Event) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//FamilyBot//CalDAV//EN")

	vevent := ical.NewEvent()
	vevent.Props.SetText(ical.PropUID, event.UID)
	vevent.Props.SetText(ical.PropSummary, event.Summary)

	if event.Description != "" {
		vevent.Props.SetText(ical.PropDescription, event.Description)
	}
	if event.Location != "" {
		vevent.Props.SetText(ical.PropLocation, event.Location)
	}

	// Set times - convert to UTC to avoid timezone issues
	if event.AllDay {
		vevent.Props.SetDate(ical.PropDateTimeStart, event.StartTime)
		if !event.EndTime.IsZero() {
			vevent.Props.SetDate(ical.PropDateTimeEnd, event.EndTime)
		}
	} else {
		// Convert to UTC explicitly - iCalendar will use Z suffix
		vevent.Props.SetDateTime(ical.PropDateTimeStart, event.StartTime.UTC())
		if !event.EndTime.IsZero() {
			vevent.Props.SetDateTime(ical.PropDateTimeEnd, event.EndTime.UTC())
		}
	}

	// Add recurrence rule if present
	if event.RRule != "" {
		vevent.Props.SetText(ical.PropRecurrenceRule, event.RRule)
	}

	// Add creation timestamp
	vevent.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())

	cal.Children = append(cal.Children, vevent.Component)
	return cal
}

// generateUID generates a unique event ID
func generateUID() string {
	return fmt.Sprintf("%d-%d@familybot", time.Now().UnixNano(), time.Now().Unix())
}

// SerializeCalendar converts calendar to string (for debugging)
func SerializeCalendar(cal *ical.Calendar) string {
	var buf bytes.Buffer
	enc := ical.NewEncoder(&buf)
	_ = enc.Encode(cal)
	return buf.String()
}
