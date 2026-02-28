package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

type HistoryItem struct {
	State string    `json:"state"`
	From  datetime  `json:"from"`
	To    *datetime `json:"to,omitempty"`
}

type GridHistoryService struct {
	mu            sync.Mutex
	jsonDbPath    string
	historyWindow time.Duration

	state       []HistoryItem
	subscribers []func(state []HistoryItem)
}

func NewGridHistoryService(
	jsonDbPath string,
	historyWindow time.Duration,
) *GridHistoryService {
	service := &GridHistoryService{
		jsonDbPath:    jsonDbPath,
		historyWindow: historyWindow,
		state:         []HistoryItem{},
		subscribers:   make([]func(state []HistoryItem), 0),
	}
	return service
}

func (service *GridHistoryService) Start(subs []func(state []HistoryItem)) {
	service.readDb()
	service.subscribers = subs
}

func (service *GridHistoryService) State() []HistoryItem {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.state
}

func (service *GridHistoryService) OnHistoryUpdate(ctx context.Context) func(state string) {
	return func(state string) {
		service.mu.Lock()
		if len(service.state) > 0 && service.state[len(service.state)-1].State == state {
			service.mu.Unlock()
			return
		}
		if len(service.state) == 0 {
			service.state = append(service.state, HistoryItem{
				State: state,
				From:  datetime{nowKyiv()},
			})
		} else {
			service.state[len(service.state)-1].To = &datetime{nowKyiv()}
			service.state = append(service.state, HistoryItem{
				State: state,
				From:  datetime{nowKyiv()},
			})
		}
		service.removeOldHistoryItems()
		subscriberCopy := make([]HistoryItem, len(service.state))
		copy(subscriberCopy, service.state)
		dbSnapshot, _ := json.MarshalIndent(service.state, "", " ")
		service.mu.Unlock()

		for _, handler := range service.subscribers {
			go handler(subscriberCopy)
		}
		err := service.writeSnapshotToDb(dbSnapshot)
		if err != nil {
			slog.Error("history db write failed", "service", "history", "error", err)
			sentry.CaptureException(err)
		}
	}
}

func (service *GridHistoryService) readDb() {
	data, err := os.Open(service.jsonDbPath)
	if err == nil {
		defer data.Close()
	}
	if err != nil {
		slog.Error("history db read failed, falling back to empty state", "service", "history", "error", err)
		service.state = []HistoryItem{}
		return
	}

	var state []HistoryItem
	err = json.NewDecoder(data).Decode(&state)
	if err != nil {
		slog.Error("history db parse failed, falling back to empty state", "service", "history", "error", err)
		service.state = []HistoryItem{}
		return
	}

	service.state = state
}

func (service *GridHistoryService) removeOldHistoryItems() {
	cutoff := nowKyiv().Add(-service.historyWindow)
	fresh := []HistoryItem{}
	for _, it := range service.state {
		if it.To == nil || !it.To.Time.Before(cutoff) {
			fresh = append(fresh, it)
		}
	}
	service.state = fresh
}

func (service *GridHistoryService) writeSnapshotToDb(data []byte) error {
	tmp := service.jsonDbPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cannot write history to temp file: %v", err)
	}
	if err := os.Rename(tmp, service.jsonDbPath); err != nil {
		return fmt.Errorf("cannot rename temp file to db file: %v", err)
	}
	return nil
}
