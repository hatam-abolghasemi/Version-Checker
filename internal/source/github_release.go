package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

type GitHubRelease struct {
	client  *Client
	repo    string
	pattern *regexp.Regexp
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (s *GitHubRelease) Latest(ctx context.Context) (Release, error) {
	h := http.Header{}
	h.Set("Accept", "application/vnd.github+json")
	h.Set("X-GitHub-Api-Version", "2022-11-28")
	if s.client.GitHubToken != "" {
		h.Set("Authorization", "Bearer "+s.client.GitHubToken)
	}
	body, err := s.client.get(ctx, fmt.Sprintf("%s/repos/%s/releases?per_page=100", s.client.GitHubAPI, s.repo), h)
	if err != nil {
		return Release{}, err
	}
	var releases []ghRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return Release{}, fmt.Errorf("decode releases: %w", err)
	}
	cands := make([]candidate, 0, len(releases))
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		cands = append(cands, candidate{tag: r.TagName, url: r.HTMLURL})
	}
	return pick(cands, s.pattern)
}
