package checker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/notify"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
	"github.com/hatam-abolghasemi/Version-Checker/internal/state"
	"github.com/hatam-abolghasemi/Version-Checker/internal/version"
)

type Target struct {
	Tool   config.Tool
	Source source.Source
}

type Checker struct {
	Targets     []Target
	Notifier    notify.Notifier
	VerifyDelay time.Duration
	Parallelism int
	// FailThreshold is how many consecutive failed fetches it takes to send a
	// "Failing" alert, so one network blip doesn't page anyone. Minimum 1.
	FailThreshold int
	Now           func() time.Time
	Log           *slog.Logger
}

type result struct {
	release source.Release
	err     error
}

// Run checks every target once. State is updated in place as messages are
// delivered, so on error the caller should still save it: whatever was sent
// is recorded and whatever wasn't is retried next run.
func (c *Checker) Run(ctx context.Context, st state.State) error {
	now := c.Now().UTC().Format(time.RFC3339)
	threshold := max(c.FailThreshold, 1)
	results := c.fetchAll(ctx, c.Targets)

	var (
		pending   []pendingUpdate
		failures  []notify.Failure
		recovered []config.Tool
	)

	for i, t := range c.Targets {
		name := t.Tool.Name
		res := results[i]
		entry := st[name]

		if res.err != nil {
			c.Log.Warn("fetch failed", "tool", name, "err", res.err)
			if entry == nil {
				entry = &state.Entry{}
				st[name] = entry
			}
			if entry.FailingSince != "" {
				continue // already alerted
			}
			entry.Failures++
			if entry.Failures >= threshold {
				failures = append(failures, notify.Failure{Tool: t.Tool, Error: truncate(res.err.Error(), 300)})
			}
			continue
		}

		c.Log.Info("fetched", "tool", name, "version", res.release.Version, "url", res.release.URL)

		if entry == nil {
			entry = &state.Entry{}
			st[name] = entry
		}
		entry.Failures = 0
		switch {
		case entry.Version == "":
			entry.Version, entry.URL, entry.UpdatedAt = res.release.Version, res.release.URL, now
			c.Log.Info("seeded", "tool", name, "version", res.release.Version)
		case version.Compare(res.release.Version, entry.Version) > 0:
			pending = append(pending, pendingUpdate{target: t, release: res.release})
		case entry.URL == "" && res.release.Version == entry.Version:
			entry.URL = res.release.URL
		}
		// FailingSince is cleared only once the recovery message is delivered.
		if entry.FailingSince != "" {
			recovered = append(recovered, t.Tool)
		}
	}

	updates := c.verify(ctx, st, pending)
	if err := c.send(ctx, notify.Report{Updates: updates}, func(r notify.Report) {
		for _, u := range r.Updates {
			e := st[u.Tool.Name]
			e.Version, e.URL, e.UpdatedAt = u.New.Version, u.New.URL, now
		}
	}); err != nil {
		return fmt.Errorf("send updates: %w", err)
	}

	if err := c.send(ctx, notify.Report{Failures: failures, Recovered: recovered}, func(r notify.Report) {
		for _, f := range r.Failures {
			st[f.Tool.Name].FailingSince = now
		}
		for _, t := range r.Recovered {
			st[t.Name].FailingSince = ""
		}
	}); err != nil {
		c.Log.Error("send alerts, retrying next run", "err", err)
	}
	return nil
}

// send delivers a report, split into several messages if the notifier has a
// size limit, and calls onSent after each message that was delivered.
func (c *Checker) send(ctx context.Context, rep notify.Report, onSent func(notify.Report)) error {
	if rep.Empty() {
		return nil
	}
	chunks := []notify.Report{rep}
	if s, ok := c.Notifier.(notify.Splitter); ok {
		var err error
		if chunks, err = s.Split(rep); err != nil {
			return err
		}
	}
	for i, chunk := range chunks {
		if err := c.Notifier.Notify(ctx, chunk); err != nil {
			if len(chunks) > 1 {
				return fmt.Errorf("message %d of %d: %w", i+1, len(chunks), err)
			}
			return err
		}
		onSent(chunk)
	}
	return nil
}

type pendingUpdate struct {
	target  Target
	release source.Release
}

func (c *Checker) verify(ctx context.Context, st state.State, pending []pendingUpdate) []notify.Update {
	if len(pending) == 0 {
		return nil
	}
	if c.VerifyDelay > 0 {
		c.Log.Info("verifying", "tools", len(pending), "delay", c.VerifyDelay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.VerifyDelay):
		}
	}
	targets := make([]Target, len(pending))
	for i, p := range pending {
		targets[i] = p.target
	}
	again := c.fetchAll(ctx, targets)

	var updates []notify.Update
	for i, p := range pending {
		name := p.target.Tool.Name
		if again[i].err != nil || again[i].release.Version != p.release.Version {
			c.Log.Warn("not confirmed, retrying next run", "tool", name, "first", p.release.Version, "second", again[i].release.Version, "err", again[i].err)
			continue
		}
		c.Log.Info("confirmed", "tool", name, "version", p.release.Version)
		updates = append(updates, notify.Update{Tool: p.target.Tool, Old: st[name].Version, New: again[i].release})
	}
	return updates
}

func (c *Checker) fetchAll(ctx context.Context, targets []Target) []result {
	n := c.Parallelism
	if n < 1 {
		n = 1
	}
	out := make([]result, len(targets))
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rel, err := t.Source.Latest(ctx)
			out[i] = result{release: rel, err: err}
		}()
	}
	wg.Wait()
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
