package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/alex-popov-tech/acapulko/src/logging"
	"github.com/alex-popov-tech/acapulko/src/server"
)

var log = logging.New("homeassistant")

type WebhookPayload struct {
	State   string `json:"state"`
	Hours   int    `json:"hours"`
	Minutes int    `json:"minutes"`
	Seconds int    `json:"seconds"`
}

// returns grid state as "on" or "off"
func GetGridState(ctx context.Context, homeAssistantURL, token string) (string, error) {
	client := http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", homeAssistantURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("cannot construct HA request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)

	res, err := client.Do(req) //nolint:gosec // URL is from server config, not user input
	if err != nil {
		return "", fmt.Errorf("failed to GET grid state: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code from HA: %d", res.StatusCode)
	}

	var body struct {
		State string `json:"state"`
	}

	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode HA response: %w", err)
	}

	if body.State != "on" && body.State != "off" {
		return "", fmt.Errorf("unexpected grid state value: %s", body.State)
	}

	return body.State, nil
}

func Attach(
	s *server.Server,
	homeHomeAssistantURL, token string,
	updatesChannel chan<- WebhookPayload,
) (initial WebhookPayload, err error) {
	prevState, err := GetGridState(context.Background(), homeHomeAssistantURL, token)
	if err != nil {
		return WebhookPayload{}, fmt.Errorf("failed to get initial grid state: %w", err)
	}
	prev := WebhookPayload{State: prevState}

	updateRes := debounce(time.Second*10, func(payload WebhookPayload) {
		if prev.State != payload.State {
			prev = payload
			updatesChannel <- payload
		}
	})

	s.POST("/api/webhook/grid", func(c *echo.Context) error {
		var payload WebhookPayload
		if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
			log().Warn("webhook rejected: invalid json", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		}
		if payload.State != "on" && payload.State != "off" {
			log().Warn("webhook rejected: invalid state", "state", payload.State)
			return c.JSON(
				http.StatusBadRequest,
				map[string]string{"error": "state must be 'on' or 'off'"},
			)
		}

		updateRes(payload)

		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return prev, nil
}

func debounce(
	timeout time.Duration,
	action func(WebhookPayload),
) func(WebhookPayload) {
	var mu sync.Mutex
	var timer *time.Timer
	return func(payload WebhookPayload) {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(timeout, func() {
			action(payload)
		})
	}
}
