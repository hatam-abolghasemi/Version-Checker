package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
)

const DefaultPattern = `^v?[0-9]+(\.[0-9]+)+$`

const (
	TypeGitHubRelease = "github-release"
	TypeGitHubTag     = "github-tag"
	TypeWebpage       = "webpage"
)

type Config struct {
	Defaults Defaults `yaml:"defaults"`
	Telegram Telegram `yaml:"telegram"`
	Tools    []Tool   `yaml:"tools"`
}

type Defaults struct {
	Pattern string `yaml:"pattern"`
}

type Telegram struct {
	Template string `yaml:"template"`
}

type Tool struct {
	Name        string            `yaml:"name"`
	DisplayName string            `yaml:"display_name"`
	Description string            `yaml:"description"`
	Homepage    string            `yaml:"homepage"`
	Labels      []string          `yaml:"labels"`
	Meta        map[string]string `yaml:"meta"`
	Source      Source            `yaml:"source"`
}

type Source struct {
	Type    string `yaml:"type"`
	Repo    string `yaml:"repo"`
	URL     string `yaml:"url"`
	Pattern string `yaml:"pattern"`
}

func (t Tool) Title() string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Name
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Defaults.Pattern == "" {
		cfg.Defaults.Pattern = DefaultPattern
	}
	for i := range cfg.Tools {
		if cfg.Tools[i].Source.Pattern == "" && cfg.Tools[i].Source.Type != TypeWebpage {
			cfg.Tools[i].Source.Pattern = cfg.Defaults.Pattern
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config:\n%w", err)
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	var errs []error
	seen := map[string]bool{}
	for i, t := range c.Tools {
		if t.Name == "" {
			errs = append(errs, fmt.Errorf("tools[%d]: name is required", i))
			continue
		}
		where := fmt.Sprintf("tool %q", t.Name)
		if seen[t.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate name", where))
		}
		seen[t.Name] = true

		switch t.Source.Type {
		case TypeGitHubRelease, TypeGitHubTag:
			if t.Source.Repo == "" {
				errs = append(errs, fmt.Errorf("%s: source.repo is required", where))
			}
		case TypeWebpage:
			if t.Source.URL == "" {
				errs = append(errs, fmt.Errorf("%s: source.url is required", where))
			}
			if t.Source.Pattern == "" {
				errs = append(errs, fmt.Errorf("%s: source.pattern is required", where))
				continue
			}
		default:
			errs = append(errs, fmt.Errorf("%s: unknown source.type %q", where, t.Source.Type))
			continue
		}

		re, err := regexp.Compile(t.Source.Pattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: source.pattern: %w", where, err))
			continue
		}
		if t.Source.Type == TypeWebpage && re.NumSubexp() != 1 {
			errs = append(errs, fmt.Errorf("%s: webpage pattern needs exactly one capture group", where))
		}
	}
	return errors.Join(errs...)
}
