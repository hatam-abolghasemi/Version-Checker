package notify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
)

type Update struct {
	Tool config.Tool
	Old  string
	New  source.Release
}

type Failure struct {
	Tool  config.Tool
	Error string
}

type Report struct {
	Updates   []Update
	Failures  []Failure
	Recovered []config.Tool
}

func (r Report) Empty() bool {
	return len(r.Updates) == 0 && len(r.Failures) == 0 && len(r.Recovered) == 0
}

type Notifier interface {
	Notify(ctx context.Context, r Report) error
}

// Splitter is implemented by notifiers with a message size limit. Split breaks
// a report into reports that each fit in one message, keeping item order.
// The checker sends them one by one and records progress after each.
type Splitter interface {
	Split(r Report) ([]Report, error)
}

const DefaultTemplate = `
{{- if .Updates }}🆕 <b>New versions</b>
{{ range .Updates }}
<b>{{ .Tool.Title }}</b>: {{ .Old }} → <a href="{{ .New.URL }}">{{ .New.Version }}</a>
{{- end }}{{ end }}
{{- if .Failures }}⚠️ <b>Failing</b>
{{ range .Failures }}
<b>{{ .Tool.Title }}</b>: {{ .Error }}
{{- end }}{{ end }}
{{- if .Recovered }}{{ if .Failures }}

{{ end }}✅ <b>Recovered</b>: {{ range $i, $t := .Recovered }}{{ if $i }}, {{ end }}{{ $t.Title }}{{ end }}{{ end }}
`

type Renderer struct {
	tmpl *template.Template
}

func NewRenderer(text string) (*Renderer, error) {
	if strings.TrimSpace(text) == "" {
		text = DefaultTemplate
	}
	t, err := template.New("message").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse telegram template: %w", err)
	}
	return &Renderer{tmpl: t}, nil
}

func (r *Renderer) Render(rep Report) (string, error) {
	var buf bytes.Buffer
	if err := r.tmpl.Execute(&buf, rep); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

// Split packs the report's items, in order, into as few reports as possible
// whose rendered text is at most limit UTF-16 code units (how Telegram counts).
func (r *Renderer) Split(rep Report, limit int) ([]Report, error) {
	if rep.Empty() {
		return nil, nil
	}
	if text, err := r.Render(rep); err != nil {
		return nil, err
	} else if utf16Len(text) <= limit {
		return []Report{rep}, nil
	}

	var items []func(*Report)
	for _, u := range rep.Updates {
		items = append(items, func(r *Report) { r.Updates = append(r.Updates, u) })
	}
	for _, f := range rep.Failures {
		items = append(items, func(r *Report) { r.Failures = append(r.Failures, f) })
	}
	for _, t := range rep.Recovered {
		items = append(items, func(r *Report) { r.Recovered = append(r.Recovered, t) })
	}

	var out []Report
	var cur Report
	for _, add := range items {
		next := cur.clone()
		add(&next)
		text, err := r.Render(next)
		if err != nil {
			return nil, err
		}
		if utf16Len(text) <= limit {
			cur = next
			continue
		}
		if cur.Empty() {
			return nil, fmt.Errorf("a single item renders to %d characters, over the %d limit", utf16Len(text), limit)
		}
		out = append(out, cur)
		cur = Report{}
		add(&cur)
		if text, err := r.Render(cur); err != nil {
			return nil, err
		} else if utf16Len(text) > limit {
			return nil, fmt.Errorf("a single item renders to %d characters, over the %d limit", utf16Len(text), limit)
		}
	}
	if !cur.Empty() {
		out = append(out, cur)
	}
	return out, nil
}

func (r Report) clone() Report {
	return Report{
		Updates:   append([]Update(nil), r.Updates...),
		Failures:  append([]Failure(nil), r.Failures...),
		Recovered: append([]config.Tool(nil), r.Recovered...),
	}
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

type Stdout struct {
	Renderer *Renderer
	W        io.Writer
}

func (s *Stdout) Notify(_ context.Context, rep Report) error {
	msg, err := s.Renderer.Render(rep)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.W, "--- message ---\n%s\n---------------\n", msg)
	return err
}
