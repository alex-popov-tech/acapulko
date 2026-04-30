package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/alex-popov-tech/acapulko/src/logging"
)

type Config struct {
	Port string

	HABaseURL string
	HAToken   string
	HAEntity  string

	DTEKBaseURL      string
	DTEKRegion       string
	DTEKCity         string
	DTEKStreet       string
	DTEKBuilding     string
	DTEKPollInterval time.Duration

	TGBaseURL  string
	TGBotToken string
	TGChatID   string

	LogLevel string
}

var (
	log = logging.New("config")
	cfg *Config
)

func Load() (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}

	err := godotenv.Load()
	if err != nil {
		log().Info("failed to load .env file", "error", err)
	}
	required := []string{
		"PORT",
		"HA_BASE_URL",
		"HA_TOKEN",
		"HA_ENTITY",
		"DTEK_BASE_URL",
		"DTEK_REGION",
		"DTEK_CITY",
		"DTEK_STREET",
		"DTEK_BUILDING",
		"DTEK_POLL_INTERVAL",
		"TG_BOT_TOKEN",
		"TG_CHAT_ID",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"missing required environment variables: %s",
			strings.Join(missing, ", "),
		)
	}

	dtekPoll, err := time.ParseDuration(os.Getenv("DTEK_POLL_INTERVAL"))
	if err != nil {
		return nil, fmt.Errorf("invalid duration for DTEK_POLL_INTERVAL: %w", err)
	}

	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("invalid LOG_LEVEL %q (want debug|info|warn|error)", logLevel)
	}

	tgBaseURL := os.Getenv("TG_BASE_URL")
	if tgBaseURL == "" {
		tgBaseURL = "https://api.telegram.org"
	}

	cfg = &Config{
		Port:             os.Getenv("PORT"),
		HABaseURL:        os.Getenv("HA_BASE_URL"),
		HAToken:          os.Getenv("HA_TOKEN"),
		HAEntity:         os.Getenv("HA_ENTITY"),
		DTEKBaseURL:      os.Getenv("DTEK_BASE_URL"),
		DTEKRegion:       os.Getenv("DTEK_REGION"),
		DTEKCity:         os.Getenv("DTEK_CITY"),
		DTEKStreet:       os.Getenv("DTEK_STREET"),
		DTEKBuilding:     os.Getenv("DTEK_BUILDING"),
		DTEKPollInterval: dtekPoll,
		TGBaseURL:        tgBaseURL,
		TGBotToken:       os.Getenv("TG_BOT_TOKEN"),
		TGChatID:         os.Getenv("TG_CHAT_ID"),
		LogLevel:         logLevel,
	}
	return cfg, nil
}
