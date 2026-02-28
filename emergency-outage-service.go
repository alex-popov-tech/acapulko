package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/getsentry/sentry-go"
)

type Outage struct {
	Type string    `json:"type"`
	From *datetime `json:"from"`
	To   *datetime `json:"to"`
}

type betterDtekResponse struct {
	City      string `json:"city"`
	Street    string `json:"street"`
	Buildings map[string]struct {
		Group  string  `json:"group"`
		Outage *Outage `json:"outage"`
	} `json:"buildings"`
}

type OutageService struct {
	betterDtekURL string
	region        string
	city          string
	street        string
	building      string
	pollInterval  time.Duration

	state       *Outage
	subscribers []func(o *Outage)

	client *http.Client
}

func NewOutageService(
	betterDtekBaseURL, region, city, street, building string,
	pollInterval time.Duration,
) *OutageService {
	return &OutageService{
		betterDtekURL: betterDtekBaseURL,
		region:        region,
		city:          city,
		street:        street,
		building:      building,
		pollInterval:  pollInterval,
		subscribers:   make([]func(o *Outage), 0),
		client:        &http.Client{Timeout: 10 * time.Second},
	}
}

func (service *OutageService) Start(ctx context.Context, subs []func(o *Outage)) {
	service.subscribers = subs
	go func() {
		ticker := time.NewTicker(service.pollInterval)
		defer ticker.Stop()

		for {
			outage, err := service.getOutage(ctx)
			if err != nil {
				slog.Error("outage poll failed", "service", "outage", "error", err)
				sentry.CaptureException(err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
				}
				continue
			}
			service.onTick(outage)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (o *Outage) Equal(other *Outage) bool {
	if o == other {
		return true
	}
	if o == nil || other == nil {
		return false
	}
	return o.Type == other.Type &&
		o.From.Equal(other.From) &&
		o.To.Equal(other.To)
}

func (service *OutageService) onTick(o *Outage) {
	if service.state.Equal(o) {
		return
	}
	service.state = o
	for _, handler := range service.subscribers {
		go handler(o)
	}
}

func (service *OutageService) getOutage(ctx context.Context) (*Outage, error) {
	u := fmt.Sprintf(
		"%s?region=%s&city=%s&street=%s",
		service.betterDtekURL,
		url.QueryEscape(service.region),
		url.QueryEscape(service.city),
		url.QueryEscape(service.street),
	)
	slog.Info("pulling emergency outage", "service", "outage", "url", u)
	req, err := http.NewRequestWithContext(ctx, "GET", u, http.NoBody) //nolint:gosec // URL is from config, not user input
	if err != nil {
		return nil, fmt.Errorf("failed to create outage request: %w", err)
	}
	res, err := service.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET streets data: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf(
			"unexpected response status code, expected 200, was %d",
			res.StatusCode,
		)
	}

	var data betterDtekResponse
	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode streets data: %w", err)
	}

	buildings := data.Buildings
	myAddress, isOk := buildings[service.building]
	if !isOk {
		return nil, fmt.Errorf(
			"cannot find required address %s in the map %v",
			service.building,
			buildings,
		)
	}
	slog.Info("finished pulling emergency outage", "service", "outage")
	return myAddress.Outage, nil
}
