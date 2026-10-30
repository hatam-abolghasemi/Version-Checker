package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
)

func testTelegram(t *testing.T, h http.HandlerFunc) *Telegram {
	t.Helper()
	baseBackoff = time.Millisecond
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r, _ := NewRenderer("")
	tg := NewTelegram("secret", "@c", r)
	tg.API = srv.URL
	return tg
}

var oneUpdate = Report{Updates: []Update{{Tool: config.Tool{Name: "a"}, Old: "1", New: source.Release{Version: "2"}}}}

func TestTelegramNoRetryOn400(t *testing.T) {
	calls := 0
	tg := testTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(400)
		fmt.Fprint(w, `{"ok":false,"description":"Bad Request: can't parse entities"}`)
	})
	err := tg.Notify(context.Background(), oneUpdate)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "parse entities") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestTelegramRetriesOn429And5xx(t *testing.T) {
	calls := 0
	tg := testTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(429)
			fmt.Fprint(w, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":0}}`)
		case 2:
			w.WriteHeader(502)
			fmt.Fprint(w, `<html>bad gateway</html>`)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	})
	if err := tg.Notify(context.Background(), oneUpdate); err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestTelegramRetryAfterPastDeadline(t *testing.T) {
	calls := 0
	tg := testTelegram(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(429)
		fmt.Fprint(w, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":600}}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tg.Notify(ctx, oneUpdate); err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestSplit(t *testing.T) {
	r, _ := NewRenderer("")
	var rep Report
	for i := range 200 {
		rep.Updates = append(rep.Updates, Update{
			Tool: config.Tool{Name: fmt.Sprintf("tool-%03d", i)},
			Old:  "1.0.0",
			New:  source.Release{Version: "1.0.1", URL: "https://github.com/o/r/releases/tag/v1.0.1"},
		})
	}
	rep.Failures = []Failure{{Tool: config.Tool{Name: "f"}, Error: strings.Repeat("e", 300)}}
	rep.Recovered = []config.Tool{{Name: "r"}}

	chunks, err := r.Split(rep, MaxMessageLen)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want several chunks, got %d", len(chunks))
	}
	var updates, failures, recovered int
	for _, c := range chunks {
		text, _ := r.Render(c)
		if n := utf16Len(text); n > MaxMessageLen {
			t.Fatalf("chunk is %d long", n)
		}
		for _, u := range c.Updates {
			if want := fmt.Sprintf("tool-%03d", updates); u.Tool.Name != want {
				t.Fatalf("order: got %s want %s", u.Tool.Name, want)
			}
			updates++
		}
		failures += len(c.Failures)
		recovered += len(c.Recovered)
	}
	if updates != 200 || failures != 1 || recovered != 1 {
		t.Fatalf("lost items: %d %d %d", updates, failures, recovered)
	}

	small, _ := r.Split(oneUpdate, MaxMessageLen)
	if len(small) != 1 {
		t.Fatalf("small report split into %d", len(small))
	}
}
