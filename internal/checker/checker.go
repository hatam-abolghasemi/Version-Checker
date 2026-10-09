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
	Now         func() time.Time
	Log         *slog.Logger
}

type result struct {
	release source.Release
	err     error
}

func (c *Checker) Run(ctx context.Context, st state.State) error {
	now := c.Now().UTC().Format(time.RFC3339)
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
			if entry == nil || entry.FailingSince == "" {
				failures = append(failures, notify.Failure{Tool: t.Tool, Error: truncate(res.err.Error(), 300)})
			}
			continue
		}

		c.Log.Info("fetched", "tool", name, "version", res.release.Version, "url", res.release.URL)

		switch {
		case entry == nil || entry.Version == "":
			seeded := &state.Entry{Version: res.release.Version, URL: res.release.URL, UpdatedAt: now}
			if entry != nil && entry.FailingSince != "" {
				recovered = append(recovered, t.Tool)
			}
			st[name] = seeded
			c.Log.Info("seeded", "tool", name, "version", res.release.Version)
			continue
		case version.Compare(res.release.Version, entry.Version) > 0:
			pending = append(pending, pendingUpdate{target: t, release: res.release})
		case entry.URL == "" && res.release.Version == entry.Version:
			entry.URL = res.release.URL
		}

		if entry.FailingSince != "" {
			entry.FailingSince = ""
			recovered = append(recovered, t.Tool)
		}
	}

	updates := c.verify(ctx, st, pending)
	if len(updates) > 0 {
		if err := c.Notifier.Notify(ctx, notify.Report{Updates: updates}); err != nil {
			return fmt.Errorf("send updates: %w", err)
		}
		for _, u := range updates {
			st[u.Tool.Name].Version = u.New.Version
			st[u.Tool.Name].URL = u.New.URL
			st[u.Tool.Name].UpdatedAt = now
		}
	}

	alerts := notify.Report{Failures: failures, Recovered: recovered}
	if alerts.Empty() {
		return nil
	}
	if err := c.Notifier.Notify(ctx, alerts); err != nil {
		c.Log.Error("send alerts", "err", err)
		return nil
	}
	for _, f := range failures {
		if st[f.Tool.Name] == nil {
			st[f.Tool.Name] = &state.Entry{}
		}
		st[f.Tool.Name].FailingSince = now
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
