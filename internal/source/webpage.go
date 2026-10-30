package source

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

var matchAll = regexp.MustCompile(``)

type Webpage struct {
	client  *Client
	url     string
	pattern *regexp.Regexp
}

func (s *Webpage) Latest(ctx context.Context) (Release, error) {
	body, err := s.client.get(ctx, s.url, nil)
	if err != nil {
		return Release{}, err
	}
	var cands []candidate
	for _, m := range s.pattern.FindAllSubmatch(body, -1) {
		v := string(m[1])
		cands = append(cands, candidate{tag: v, url: highlight(s.url, v)})
	}
	return pick(cands, matchAll)
}

func highlight(pageURL, text string) string {
	base, _, _ := strings.Cut(pageURL, "#")
	r := strings.NewReplacer("-", "%2D", ",", "%2C", "&", "%26")
	return base + "#:~:text=" + r.Replace(url.PathEscape(text))
}
