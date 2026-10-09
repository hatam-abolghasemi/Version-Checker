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
