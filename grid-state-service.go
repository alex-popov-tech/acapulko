package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
)

type homeassistantResponse struct {
	State string `json:"state"`
}

type GridStateService struct {
	homeassistantURL string
	token            string
	pollInterval     time.Duration

	state       string
	subscribers []func(state string)

	client *http.Client
}

func NewGridStateService(
	homeassistantURL, token string,
	pollInterval time.Duration,
) *GridStateService {
	return &GridStateService{
		homeassistantURL: homeassistantURL,
		token:            token,
		pollInterval:     pollInterval,
		subscribers:      make([]func(state string), 0),
		client:           &http.Client{Timeout: 10 * time.Second},
	}
}

func (service *GridStateService) Start(ctx context.Context, subs []func(state string)) {
	service.subscribers = subs
	go func() {
		ticker := time.NewTicker(service.pollInterval)
		defer ticker.Stop()
		for {
			state, err := service.getGridState(ctx)
			if err != nil {
				slog.Error("grid state poll failed", "service", "grid-state", "error", err)
				sentry.CaptureException(err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
					continue
				}
			}
			service.onTick(state)

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (service *GridStateService) onTick(state string) {
	if service.state == state {
		return
	}
	service.state = state
	for _, handler := range service.subscribers {
		go handler(state)
	}
}

func (service *GridStateService) getGridState(ctx context.Context) (string, error) {
	slog.Info("pulling grid state", "service", "grid-state", "url", service.homeassistantURL)
	req, err := http.NewRequestWithContext(ctx, "GET", service.homeassistantURL, http.NoBody) //nolint:gosec // URL is from config, not user input
	if err != nil {
		return "", fmt.Errorf("cannot construct url for homeassistant pulling: %w", err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", service.token))
	res, err := service.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to GET grid state: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf(
			"unexpected response status code, expected 200, was %d",
			res.StatusCode,
		)
	}

	var data homeassistantResponse
	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		return "", fmt.Errorf("failed to decode grid state: %w", err)
	}
	if data.State != "on" && data.State != "off" {
		return "", fmt.Errorf(
			"unexpected value in grid state found, expected on|off, was %s",
			data.State,
		)
	}
	return data.State, nil
}
