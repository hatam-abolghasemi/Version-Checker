package checker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/notify"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
	"github.com/hatam-abolghasemi/Version-Checker/internal/state"
)

type fakeSource struct {
	results []source.Release
	errs    []error
	calls   int
}

func (f *fakeSource) Latest(context.Context) (source.Release, error) {
	i := min(f.calls, len(f.results)-1)
	f.calls++
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return f.results[i], err
}

type fakeNotifier struct {
	reports []notify.Report
	err     error
}

func (f *fakeNotifier) Notify(_ context.Context, r notify.Report) error {
	if f.err != nil {
		return f.err
	}
	f.reports = append(f.reports, r)
	return nil
}

func rel(v string) source.Release { return source.Release{Version: v, URL: "u/" + v} }

func newChecker(n notify.Notifier, srcs map[string]*fakeSource) *Checker {
	c := &Checker{
		Notifier:    n,
		Parallelism: 2,
		Now:         func() time.Time { return time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC) },
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, name := range []string{"a", "b", "c"} {
		if s, ok := srcs[name]; ok {
			c.Targets = append(c.Targets, Target{Tool: config.Tool{Name: name}, Source: s})
		}
	}
	return c
}

func TestSeedSilently(t *testing.T) {
	n := &fakeNotifier{}
	st := state.State{}
	c := newChecker(n, map[string]*fakeSource{"a": {results: []source.Release{rel("1.0.0")}}})
	if err := c.Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if len(n.reports) != 0 || st["a"].Version != "1.0.0" {
		t.Fatalf("reports=%v state=%+v", n.reports, st["a"])
	}
}

func TestUpdateDowngradeAndVerify(t *testing.T) {
	n := &fakeNotifier{}
	st := state.State{
		"a": {Version: "1.0.0"},
		"b": {Version: "2.0.0"},
		"c": {Version: "3.0.0"},
	}
	c := newChecker(n, map[string]*fakeSource{
		"a": {results: []source.Release{rel("1.1.0")}},
		"b": {results: []source.Release{rel("1.9.0")}},
		"c": {results: []source.Release{rel("3.1.0"), rel("3.0.0")}},
	})
	if err := c.Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if len(n.reports) != 1 || len(n.reports[0].Updates) != 1 {
		t.Fatalf("reports=%+v", n.reports)
	}
	u := n.reports[0].Updates[0]
	if u.Tool.Name != "a" || u.Old != "1.0.0" || u.New.URL != "u/1.1.0" {
		t.Fatalf("update=%+v", u)
	}
	if st["a"].Version != "1.1.0" || st["b"].Version != "2.0.0" || st["c"].Version != "3.0.0" {
		t.Fatalf("state a=%v b=%v c=%v", st["a"], st["b"], st["c"])
	}
}

func TestNotifyFailureKeepsState(t *testing.T) {
	n := &fakeNotifier{err: errors.New("boom")}
	st := state.State{"a": {Version: "1.0.0"}}
	c := newChecker(n, map[string]*fakeSource{"a": {results: []source.Release{rel("1.1.0")}}})
	if err := c.Run(context.Background(), st); err == nil {
		t.Fatal("expected error")
	}
	if st["a"].Version != "1.0.0" {
		t.Fatalf("state changed: %+v", st["a"])
	}
}

func TestFailureAlertsOnceThenRecovery(t *testing.T) {
	n := &fakeNotifier{}
	st := state.State{"a": {Version: "1.0.0"}}
	fail := &fakeSource{results: []source.Release{{}}, errs: []error{errors.New("404")}}
	c := newChecker(n, map[string]*fakeSource{"a": fail})

	c.Run(context.Background(), st)
	c.Run(context.Background(), st)
	if len(n.reports) != 1 || len(n.reports[0].Failures) != 1 || st["a"].FailingSince == "" {
		t.Fatalf("reports=%+v state=%+v", n.reports, st["a"])
	}

	c.Targets[0].Source = &fakeSource{results: []source.Release{rel("1.0.0")}}
	c.Run(context.Background(), st)
	if len(n.reports) != 2 || len(n.reports[1].Recovered) != 1 || st["a"].FailingSince != "" {
		t.Fatalf("reports=%+v state=%+v", n.reports, st["a"])
	}
}

func TestFailureBeforeSeed(t *testing.T) {
	n := &fakeNotifier{}
	st := state.State{}
	c := newChecker(n, map[string]*fakeSource{"a": {results: []source.Release{{}}, errs: []error{errors.New("x")}}})
	c.Run(context.Background(), st)
	c.Run(context.Background(), st)
	if len(n.reports) != 1 {
		t.Fatalf("want 1 alert, got %d", len(n.reports))
	}
	c.Targets[0].Source = &fakeSource{results: []source.Release{rel("2.0.0")}}
	c.Run(context.Background(), st)
	if st["a"].Version != "2.0.0" || len(n.reports) != 2 || len(n.reports[1].Updates) != 0 {
		t.Fatalf("state=%+v reports=%+v", st["a"], n.reports)
	}
}
