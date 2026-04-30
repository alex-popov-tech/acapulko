package dtek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/alex-popov-tech/acapulko/src/logging"
)

var log = logging.New("dtek")

type Outage struct {
	Type string    `json:"type"`
	From *datetime `json:"from"`
	To   *datetime `json:"to"`
}

func (o *Outage) Equal(other *Outage) bool {
	if o == nil || other == nil {
		return o == other
	}
	return o.Type == other.Type &&
		o.From.Equal(other.From) &&
		o.To.Equal(other.To)
}

func Start(
	ctx context.Context,
	dtekBaseURL, region, city, street, building string,
	pollInterval time.Duration,
	outageUpdates chan<- *Outage,
) (*Outage, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	prevOutage, err := getOutage(ctx, client, dtekBaseURL, region, city, street, building)
	if err != nil {
		return prevOutage, err
	}

	go func() {
		log().Info("polling started", "interval", pollInterval)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log().Info("polling stopped")
				return
			case <-ticker.C:
			}

			outage, err := getOutage(ctx, client, dtekBaseURL, region, city, street, building)
			if err != nil {
				log().Error("poll failed", "error", err)
			} else if !prevOutage.Equal(outage) {
				prevOutage = outage
				outageUpdates <- outage
			}
		}
	}()

	return prevOutage, nil
}

func getOutage(
	ctx context.Context,
	client *http.Client,
	dtekURL, region, city, street, building string,
) (*Outage, error) {
	u := fmt.Sprintf(
		"%s/api/status?region=%s&city=%s&street=%s",
		dtekURL,
		url.QueryEscape(region),
		url.QueryEscape(city),
		url.QueryEscape(street),
	)
	log().Debug("polling outage", "url", u)
	req, err := http.NewRequestWithContext(ctx, "GET", u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create outage request: %w", err)
	}
	res, err := client.Do(req) //nolint:gosec // URL is from config, not user input
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

	var data struct {
		Buildings map[string]struct {
			Outage *Outage `json:"outage"`
		} `json:"buildings"`
	}

	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode streets data: %w", err)
	}

	buildings := data.Buildings
	myAddress, isOk := buildings[building]
	if !isOk {
		return nil, fmt.Errorf("cannot find required address %s in the map %v", building, buildings)
	}

	return myAddress.Outage, nil
}
