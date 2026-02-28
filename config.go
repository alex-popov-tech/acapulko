package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port string

	HABaseURL      string
	HAToken        string
	HAEntity       string
	HAPollInterval time.Duration

	DTEKBaseURL      string
	DTEKRegion       string
	DTEKCity         string
	DTEKStreet       string
	DTEKBuilding     string
	DTEKPollInterval time.Duration

	HistoryFilePath string
	HistoryWindow   time.Duration

	SentryDSN string
	SentryEnv string
}

func loadConfig() (*Config, error) {
	required := []string{
		"PORT",
		"HA_BASE_URL", "HA_TOKEN", "HA_ENTITY", "HA_POLL_INTERVAL",
		"DTEK_BASE_URL", "DTEK_REGION", "DTEK_CITY", "DTEK_STREET", "DTEK_BUILDING", "DTEK_POLL_INTERVAL",
		"HISTORY_FILE_PATH", "HISTORY_WINDOW",
		"SENTRY_DSN", "SENTRY_ENV",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	haPoll, err := time.ParseDuration(os.Getenv("HA_POLL_INTERVAL"))
	if err != nil {
		return nil, fmt.Errorf("invalid duration for HA_POLL_INTERVAL: %w", err)
	}
	dtekPoll, err := time.ParseDuration(os.Getenv("DTEK_POLL_INTERVAL"))
	if err != nil {
		return nil, fmt.Errorf("invalid duration for DTEK_POLL_INTERVAL: %w", err)
	}
	historyWindow, err := time.ParseDuration(os.Getenv("HISTORY_WINDOW"))
	if err != nil {
		return nil, fmt.Errorf("invalid duration for HISTORY_WINDOW: %w", err)
	}

	return &Config{
		Port:             os.Getenv("PORT"),
		HABaseURL:        os.Getenv("HA_BASE_URL"),
		HAToken:          os.Getenv("HA_TOKEN"),
		HAEntity:         os.Getenv("HA_ENTITY"),
		HAPollInterval:   haPoll,
		DTEKBaseURL:      os.Getenv("DTEK_BASE_URL"),
		DTEKRegion:       os.Getenv("DTEK_REGION"),
		DTEKCity:         os.Getenv("DTEK_CITY"),
		DTEKStreet:       os.Getenv("DTEK_STREET"),
		DTEKBuilding:     os.Getenv("DTEK_BUILDING"),
		DTEKPollInterval: dtekPoll,
		HistoryFilePath:  os.Getenv("HISTORY_FILE_PATH"),
		HistoryWindow:    historyWindow,
		SentryDSN:        os.Getenv("SENTRY_DSN"),
		SentryEnv:        os.Getenv("SENTRY_ENV"),
	}, nil
}
