package config

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(`
tools:
  - name: kubernetes
    display_name: Kubernetes
    labels: [k8s]
    meta: {team: platform}
    source: {type: github-release, repo: kubernetes/kubernetes}
`))
	if err != nil {
		t.Fatal(err)
	}
	k := cfg.Tools[0]
	if k.Source.Pattern != DefaultPattern || k.Title() != "Kubernetes" || k.Meta["team"] != "platform" {
		t.Fatalf("got %+v", k)
	}
}

func TestValidate(t *testing.T) {
	_, err := Parse([]byte(`
tools:
  - name: a
    source: {type: github-release}
  - name: a
    source: {type: webpage, url: "https://x", pattern: 'v[0-9]+'}
  - name: b
    source: {type: ftp}
`))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"source.repo is required", "duplicate name", "capture group", "unknown source.type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestUnknownField(t *testing.T) {
	if _, err := Parse([]byte("tools:\n  - name: a\n    sauce: {}\n")); err == nil {
		t.Fatal("expected unknown field error")
	}
}
