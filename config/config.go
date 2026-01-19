package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	TelegramToken    string
	OwnerTelegramID  int64
	PartnerTelegramID int64
	DatabasePath     string
	Timezone         *time.Location
	MorningTime      string
	EveningTime      string
	WebhookURL       string
	ServerPort       string
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

	return &Config{
		TelegramToken:    token,
		OwnerTelegramID:  ownerID,
		PartnerTelegramID: partnerID,
		DatabasePath:     dbPath,
		Timezone:         tz,
		MorningTime:      morningTime,
		EveningTime:      eveningTime,
		WebhookURL:       webhookURL,
		ServerPort:       serverPort,
	}, nil
}

func (c *Config) IsAllowedUser(telegramID int64) bool {
	return telegramID == c.OwnerTelegramID || telegramID == c.PartnerTelegramID
}
