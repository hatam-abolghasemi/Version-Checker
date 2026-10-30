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

const maxBody = 32 << 20

// open sends a request and returns the body of a 200 response, capped at
// maxBody. The caller must close it.
func (c *Client) open(ctx context.Context, method, url string, header http.Header, body io.Reader) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
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
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s", method, url, resp.Status)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, maxBody), resp.Body}, nil
}

func (c *Client) get(ctx context.Context, url string, header http.Header) ([]byte, error) {
	rc, err := c.open(ctx, http.MethodGet, url, header, nil)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (c *Client) githubHeader() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/vnd.github+json")
	h.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.GitHubToken != "" {
		h.Set("Authorization", "Bearer "+c.GitHubToken)
	}
	return h
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
