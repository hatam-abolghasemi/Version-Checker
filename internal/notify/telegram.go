package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Telegram struct {
	Token    string
	ChatID   string
	Renderer *Renderer
	HTTP     *http.Client
	API      string
}

func NewTelegram(token, chatID string, r *Renderer) *Telegram {
	return &Telegram{
		Token:    token,
		ChatID:   chatID,
		Renderer: r,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		API:      "https://api.telegram.org",
	}
}

type sendMessage struct {
	ChatID             string             `json:"chat_id"`
	Text               string             `json:"text"`
	ParseMode          string             `json:"parse_mode"`
	LinkPreviewOptions linkPreviewOptions `json:"link_preview_options"`
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func (t *Telegram) Notify(ctx context.Context, rep Report) error {
	text, err := t.Renderer.Render(rep)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(sendMessage{
		ChatID:             t.ChatID,
		Text:               text,
		ParseMode:          "HTML",
		LinkPreviewOptions: linkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if lastErr = t.send(ctx, payload); lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return lastErr
}

func (t *Telegram) send(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", t.API, t.Token), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %w", redact(err, t.Token))
	}
	defer resp.Body.Close()
	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram: %s: decode response: %w", resp.Status, err)
	}
	if !r.OK {
		return fmt.Errorf("telegram: %s: %s", resp.Status, r.Description)
	}
	return nil
}

func redact(err error, secret string) error {
	if secret == "" {
		return err
	}
	return fmt.Errorf("%s", bytes.ReplaceAll([]byte(err.Error()), []byte(secret), []byte("***")))
}
