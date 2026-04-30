package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alex-popov-tech/acapulko/src/logging"
)

var log = logging.New("telegram")

type Client struct {
	baseURL string
	token   string
}

func New(baseURL, token string) *Client {
	return &Client{baseURL: baseURL, token: token}
}

func (t *Client) SendMessage(chatID, message string) {
	payload, err := json.Marshal(map[string]string{"chat_id": chatID, "text": message})
	if err != nil {
		log().Error("alert: marshal failed", "error", err)
		return
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.token)
	res, err := http.Post(url, "application/json", bytes.NewReader(payload)) //nolint:gosec // URL is from trusted config, not user input
	if err != nil {
		log().Error("alert: post failed", "error", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		log().Error("alert: bad status", "status", res.StatusCode, "body", string(body))
		return
	}
}
