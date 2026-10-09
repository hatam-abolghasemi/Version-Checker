package source

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type GitHubTag struct {
	client  *Client
	repo    string
	pattern *regexp.Regexp
}

func (s *GitHubTag) Latest(ctx context.Context) (Release, error) {
	body, err := s.client.get(ctx, fmt.Sprintf("%s/%s.git/info/refs?service=git-upload-pack", s.client.GitHubWeb, s.repo), nil)
	if err != nil {
		return Release{}, err
	}
	tags, err := parseTags(body)
	if err != nil {
		return Release{}, err
	}
	cands := make([]candidate, 0, len(tags))
	for _, t := range tags {
		cands = append(cands, candidate{
			tag: t,
			url: fmt.Sprintf("%s/%s/releases/tag/%s", s.client.GitHubWeb, s.repo, url.PathEscape(t)),
		})
	}
	return pick(cands, s.pattern)
}

func parseTags(adv []byte) ([]string, error) {
	var tags []string
	for len(adv) >= 4 {
		n, err := strconv.ParseUint(string(adv[:4]), 16, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid pkt-line length %q", adv[:4])
		}
		if n == 0 {
			adv = adv[4:]
			continue
		}
		if n < 4 || int(n) > len(adv) {
			return nil, fmt.Errorf("truncated pkt-line")
		}
		line := adv[4:n]
		adv = adv[n:]

		if i := bytes.IndexByte(line, 0); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(string(line))
		if len(fields) != 2 {
			continue
		}
		ref, ok := strings.CutPrefix(fields[1], "refs/tags/")
		if !ok || strings.HasSuffix(ref, "^{}") {
			continue
		}
		tags = append(tags, ref)
	}
	return tags, nil
}
