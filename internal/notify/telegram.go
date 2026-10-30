package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// MaxMessageLen is Telegram's sendMessage text limit. It is checked against the
// raw HTML, which is always at least as long as the parsed text.
const MaxMessageLen = 4096

const maxAttempts = 3

// baseBackoff is the wait after the first failed attempt; it grows linearly.
var baseBackoff = 2 * time.Second

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

func (t *Telegram) Split(rep Report) ([]Report, error) {
	return t.Renderer.Split(rep, MaxMessageLen)
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
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// sendError says whether a failed send is worth retrying and how long
// Telegram asked us to wait.
type sendError struct {
	err        error
	retryable  bool
	retryAfter time.Duration
}

func (e *sendError) Error() string { return e.err.Error() }
func (e *sendError) Unwrap() error { return e.err }

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

	for attempt := 1; ; attempt++ {
		err := t.send(ctx, payload)
		if err == nil {
			return nil
		}
		var se *sendError
		if !errors.As(err, &se) || !se.retryable || attempt == maxAttempts {
			return err
		}
		wait := max(se.retryAfter, time.Duration(attempt)*baseBackoff)
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < wait {
			return fmt.Errorf("%w (retry in %s would pass the run deadline)", err, wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// send makes one sendMessage call. Network errors, 429 and 5xx are retryable;
// other 4xx responses (bad HTML, wrong chat, bad token) are not.
func (t *Telegram) send(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", t.API, t.Token), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return &sendError{err: fmt.Errorf("telegram: %w", redact(err, t.Token)), retryable: ctx.Err() == nil}
	}
	defer resp.Body.Close()

	retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return &sendError{err: fmt.Errorf("telegram: %s: decode response: %w", resp.Status, err), retryable: retryable}
	}
	if !r.OK {
		return &sendError{
			err:        fmt.Errorf("telegram: %s: %s", resp.Status, r.Description),
			retryable:  retryable,
			retryAfter: time.Duration(r.Parameters.RetryAfter) * time.Second,
		}
	}
	return nil
}

func redact(err error, secret string) error {
	if secret == "" {
		return err
	}
	return fmt.Errorf("%s", bytes.ReplaceAll([]byte(err.Error()), []byte(secret), []byte("***")))
}
