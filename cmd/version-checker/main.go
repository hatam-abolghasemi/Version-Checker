package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/hatam-abolghasemi/Version-Checker/internal/checker"
	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/notify"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
	"github.com/hatam-abolghasemi/Version-Checker/internal/state"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "tools.yaml", "path to the tools config")
		statePath   = flag.String("state", "state.json", "path to the state file")
		dryRun      = flag.Bool("dry-run", os.Getenv("DRY_RUN") == "1", "print messages instead of sending them")
		verifyDelay = flag.Duration("verify-delay", 20*time.Second, "wait before re-fetching a new version to confirm it")
		parallelism = flag.Int("parallelism", 6, "number of sources fetched at once")
		timeout     = flag.Duration("timeout", 4*time.Minute, "overall run timeout")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	st, err := state.Load(*statePath)
	if err != nil {
		return err
	}

	renderer, err := notify.NewRenderer(cfg.Telegram.Template)
	if err != nil {
		return err
	}
	var notifier notify.Notifier
	if *dryRun {
		notifier = &notify.Stdout{Renderer: renderer, W: os.Stdout}
	} else {
		token, chatID := os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID")
		if token == "" || chatID == "" {
			return errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required (or use -dry-run)")
		}
		notifier = notify.NewTelegram(token, chatID, renderer)
	}

	client := source.NewClient("version-checker/"+version, os.Getenv("GITHUB_TOKEN"))
	keep := map[string]bool{}
	targets := make([]checker.Target, 0, len(cfg.Tools))
	for _, t := range cfg.Tools {
		src, err := source.New(client, t.Source)
		if err != nil {
			return fmt.Errorf("tool %q: %w", t.Name, err)
		}
		targets = append(targets, checker.Target{Tool: t, Source: src})
		keep[t.Name] = true
	}
	st.Prune(keep)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, *timeout)
	defer cancelTimeout()

	c := &checker.Checker{
		Targets:     targets,
		Notifier:    notifier,
		VerifyDelay: *verifyDelay,
		Parallelism: *parallelism,
		Now:         time.Now,
		Log:         log,
	}
	if err := c.Run(ctx, st); err != nil {
		return fmt.Errorf("%w (state not saved, will retry next run)", err)
	}
	return st.Save(*statePath)
}
