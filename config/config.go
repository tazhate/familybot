package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	TelegramToken     string
	OwnerTelegramID   int64
	PartnerTelegramID int64
	GroupChatID       int64 // Group chat for shared messages (e.g., daily quotes)
	DatabasePath      string
	Timezone          *time.Location
	MorningTime       string
	EveningTime       string
	WebhookURL        string
	ServerPort        string
	APIUsername       string
	APIPassword       string
	// Debt Manager integration
	DebtManagerURL   string
	DebtManagerToken string
	// Apple Calendar (CalDAV) integration
	CalDAVURL        string
	CalDAVUsername   string
	CalDAVPassword   string
	CalDAVCalendarID string
	// Todoist integration
	TodoistToken            string
	TodoistProjectID        string
	TodoistSectionID        string // Owner's section
	TodoistPartnerSectionID string // Partner's section
}

func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	ownerID, err := strconv.ParseInt(os.Getenv("OWNER_TELEGRAM_ID"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("OWNER_TELEGRAM_ID is required and must be a number")
	}

	var partnerID int64
	if p := os.Getenv("PARTNER_TELEGRAM_ID"); p != "" {
		partnerID, _ = strconv.ParseInt(p, 10, 64)
	}

	var groupChatID int64
	if g := os.Getenv("GROUP_CHAT_ID"); g != "" {
		groupChatID, _ = strconv.ParseInt(g, 10, 64)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/familybot.db"
	}

	tzName := os.Getenv("TIMEZONE")
	if tzName == "" {
		tzName = "Europe/Moscow"
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("invalid TIMEZONE: %w", err)
	}

	morningTime := os.Getenv("MORNING_TIME")
	if morningTime == "" {
		morningTime = "09:00"
	}

	eveningTime := os.Getenv("EVENING_TIME")
	if eveningTime == "" {
		eveningTime = "21:00"
	}

	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "https://family.tazhate.com"
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080"
	}

	apiUsername := os.Getenv("API_USERNAME")
	apiPassword := os.Getenv("API_PASSWORD")

	// Debt Manager integration (optional)
	debtManagerURL := os.Getenv("DEBT_MANAGER_URL")
	debtManagerToken := os.Getenv("DEBT_MANAGER_TOKEN")

	// Apple Calendar (CalDAV) integration (optional)
	caldavURL := os.Getenv("CALDAV_URL")
	if caldavURL == "" {
		caldavURL = "https://caldav.icloud.com"
	}
	caldavUsername := os.Getenv("CALDAV_USERNAME")
	caldavPassword := os.Getenv("CALDAV_PASSWORD")
	caldavCalendarID := os.Getenv("CALDAV_CALENDAR_ID")

	// Todoist integration (optional)
	todoistToken := os.Getenv("TODOIST_TOKEN")
	todoistProjectID := os.Getenv("TODOIST_PROJECT_ID")
	todoistSectionID := os.Getenv("TODOIST_SECTION_ID")
	todoistPartnerSectionID := os.Getenv("TODOIST_PARTNER_SECTION_ID")

	return &Config{
		TelegramToken:     token,
		OwnerTelegramID:   ownerID,
		PartnerTelegramID: partnerID,
		GroupChatID:       groupChatID,
		DatabasePath:      dbPath,
		Timezone:          tz,
		MorningTime:       morningTime,
		EveningTime:       eveningTime,
		WebhookURL:        webhookURL,
		ServerPort:        serverPort,
		APIUsername:       apiUsername,
		APIPassword:       apiPassword,
		DebtManagerURL:   debtManagerURL,
		DebtManagerToken: debtManagerToken,
		CalDAVURL:        caldavURL,
		CalDAVUsername:   caldavUsername,
		CalDAVPassword:   caldavPassword,
		CalDAVCalendarID: caldavCalendarID,
		TodoistToken:            todoistToken,
		TodoistProjectID:        todoistProjectID,
		TodoistSectionID:        todoistSectionID,
		TodoistPartnerSectionID: todoistPartnerSectionID,
	}, nil
}

func (c *Config) IsAllowedUser(telegramID int64) bool {
	return telegramID == c.OwnerTelegramID || telegramID == c.PartnerTelegramID
}
