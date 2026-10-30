package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// releasesPerFetch is how many of the most recently created releases are
// considered. Projects that ship several patch lines at once still fit.
const releasesPerFetch = 100

type GitHubRelease struct {
	client  *Client
	repo    string
	pattern *regexp.Regexp
}

// Latest uses the GraphQL API when a token is set, because it returns only
// the fields needed (a few KB) instead of every release's changelog body
// (several MB for large projects). GraphQL requires a token, so without one it
// falls back to the REST API, decoded as a stream.
func (s *GitHubRelease) Latest(ctx context.Context) (Release, error) {
	var (
		cands []candidate
		err   error
	)
	if s.client.GitHubToken != "" {
		cands, err = s.graphql(ctx)
	} else {
		cands, err = s.rest(ctx)
	}
	if err != nil {
		return Release{}, err
	}
	return pick(cands, s.pattern)
}

const releasesQuery = `query($owner: String!, $name: String!, $n: Int!) {
  repository(owner: $owner, name: $name) {
    releases(first: $n, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes { tagName url isDraft isPrerelease }
    }
  }
}`

func (s *GitHubRelease) graphql(ctx context.Context) ([]candidate, error) {
	owner, name, ok := strings.Cut(s.repo, "/")
	if !ok {
		return nil, fmt.Errorf("invalid repo %q", s.repo)
	}
	payload, err := json.Marshal(map[string]any{
		"query":     releasesQuery,
		"variables": map[string]any{"owner": owner, "name": name, "n": releasesPerFetch},
	})
	if err != nil {
		return nil, err
	}
	h := s.client.githubHeader()
	h.Set("Content-Type", "application/json")
	rc, err := s.client.open(ctx, http.MethodPost, s.client.GitHubAPI+"/graphql", h, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var resp struct {
		Data struct {
			Repository *struct {
				Releases struct {
					Nodes []struct {
						TagName      string `json:"tagName"`
						URL          string `json:"url"`
						IsDraft      bool   `json:"isDraft"`
						IsPrerelease bool   `json:"isPrerelease"`
					} `json:"nodes"`
				} `json:"releases"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rc).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
	}
	if resp.Data.Repository == nil {
		return nil, errors.New("graphql: repository not found")
	}
	nodes := resp.Data.Repository.Releases.Nodes
	cands := make([]candidate, 0, len(nodes))
	for _, r := range nodes {
		if r.IsDraft || r.IsPrerelease {
			continue
		}
		cands = append(cands, candidate{tag: r.TagName, url: r.URL})
	}
	return cands, nil
}

func (s *GitHubRelease) rest(ctx context.Context) ([]candidate, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", s.client.GitHubAPI, s.repo, releasesPerFetch)
	rc, err := s.client.open(ctx, http.MethodGet, url, s.client.githubHeader(), nil)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// Decode one release at a time so the changelog bodies are never all held
	// in memory at once; unknown fields like "body" are skipped.
	dec := json.NewDecoder(rc)
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return nil, fmt.Errorf("decode releases: expected a JSON array (%v)", err)
	}
	var cands []candidate
	for dec.More() {
		var r struct {
			TagName    string `json:"tag_name"`
			HTMLURL    string `json:"html_url"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("decode releases: %w", err)
		}
		if r.Draft || r.Prerelease {
			continue
		}
		cands = append(cands, candidate{tag: r.TagName, url: r.HTMLURL})
	}
	return cands, nil
}
