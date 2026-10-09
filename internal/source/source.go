package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/version"
)

var ErrNoMatch = errors.New("no version matched the pattern")

type Release struct {
	Version string `json:"version"`
	Tag     string `json:"tag,omitempty"`
	URL     string `json:"url,omitempty"`
}

type Source interface {
	Latest(ctx context.Context) (Release, error)
}

type Client struct {
	HTTP        *http.Client
	UserAgent   string
	GitHubToken string
	GitHubAPI   string
	GitHubWeb   string
}

func NewClient(userAgent, githubToken string) *Client {
	return &Client{
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		UserAgent:   userAgent,
		GitHubToken: githubToken,
		GitHubAPI:   "https://api.github.com",
		GitHubWeb:   "https://github.com",
	}
}

func New(c *Client, sc config.Source) (Source, error) {
	re, err := regexp.Compile(sc.Pattern)
	if err != nil {
		return nil, err
	}
	switch sc.Type {
	case config.TypeGitHubRelease:
		return &GitHubRelease{client: c, repo: sc.Repo, pattern: re}, nil
	case config.TypeGitHubTag:
		return &GitHubTag{client: c, repo: sc.Repo, pattern: re}, nil
	case config.TypeWebpage:
		return &Webpage{client: c, url: sc.URL, pattern: re}, nil
	}
	return nil, fmt.Errorf("unknown source type %q", sc.Type)
}

func (c *Client) get(ctx context.Context, url string, header http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}

type candidate struct {
	tag string
	url string
}

func pick(cands []candidate, pattern *regexp.Regexp) (Release, error) {
	var best Release
	for _, c := range cands {
		if !pattern.MatchString(c.tag) {
			continue
		}
		v := version.Normalize(c.tag)
		if v == "" {
			continue
		}
		if best.Version == "" || version.Compare(v, best.Version) > 0 {
			best = Release{Version: v, Tag: c.tag, URL: c.url}
		}
	}
	if best.Version == "" {
		return Release{}, ErrNoMatch
	}
	return best, nil
}
