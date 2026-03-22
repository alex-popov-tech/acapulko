package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

type Telegram struct {
	config *Config
}

func (t *Telegram) sendMessage(message string) {
	payload, err := json.Marshal(map[string]string{"chat_id": t.config.TGChatID, "text": message})
	if err != nil {
		slog.Error("telegram alert failed", "error", err)
		return
	}

	res, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.config.TGBotToken),
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil || res.StatusCode != 200 {
		slog.Error("telegram alert failed", "error", err)
	}
	if err == nil {
		defer res.Body.Close()
	}
}
