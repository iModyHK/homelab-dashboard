package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Telegram struct {
	endpoint string
	chatID   string
	http     *http.Client
}

func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		endpoint: "https://api.telegram.org/bot" + botToken + "/sendMessage",
		chatID:   chatID,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *Telegram) Name() string {
	return "telegram"
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	payload, _ := json.Marshal(map[string]any{
		"chat_id":                  t.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return errors.New("telegram api unreachable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Description string `json:"description"`
		}
		json.Unmarshal(body, &apiErr)
		if apiErr.Description == "" {
			apiErr.Description = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("telegram returned %d: %s", resp.StatusCode, apiErr.Description)
	}
	return nil
}
