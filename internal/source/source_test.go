package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
)

func pkt(s string) string {
	return fmt.Sprintf("%04x%s", len(s)+4, s)
}

func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("test", "")
	c.GitHubAPI = srv.URL
	c.GitHubWeb = srv.URL
	return c
}

func TestGitHubRelease(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v1.38.0-rc.0","html_url":"u-rc","prerelease":true},
			{"tag_name":"v1.36.9","html_url":"u-old"},
			{"tag_name":"v1.37.1","html_url":"u-new"},
			{"tag_name":"v1.99.0","html_url":"u-draft","draft":true}
		]`)
	}))
	src, _ := New(c, config.Source{Type: config.TypeGitHubRelease, Repo: "k/k", Pattern: config.DefaultPattern})
	got, err := src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.37.1" || got.URL != "u-new" || got.Tag != "v1.37.1" {
		t.Fatalf("got %+v", got)
	}
}

func TestGitHubTag(t *testing.T) {
	adv := pkt("# service=git-upload-pack\n") + "0000" +
		pkt("aaaa HEAD\x00multi_ack side-band\n") +
		pkt("bbbb refs/heads/main\n") +
		pkt("cccc refs/tags/release-3.9.6\n") +
		pkt("dddd refs/tags/release-3.9.6^{}\n") +
		pkt("eeee refs/tags/release-3.10.0-rc1\n") +
		pkt("ffff refs/tags/release-3.8.4\n") + "0000"
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/apache/zookeeper.git/info/refs") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, adv)
	}))
	src, _ := New(c, config.Source{Type: config.TypeGitHubTag, Repo: "apache/zookeeper", Pattern: `^release-[0-9]+\.[0-9]+\.[0-9]+$`})
	got, err := src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "3.9.6" || !strings.HasSuffix(got.URL, "/apache/zookeeper/releases/tag/release-3.9.6") {
		t.Fatalf("got %+v", got)
	}
}

func TestWebpage(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="proxmox-ve_9.2-1.iso"></a><a href="proxmox-ve_9.3-1.iso"></a><a href="proxmox-ve_8.4-1.iso"></a>`)
	}))
	page := c.GitHubWeb + "/iso"
	src, _ := New(c, config.Source{Type: config.TypeWebpage, URL: page, Pattern: `proxmox-ve_([0-9]+\.[0-9]+(?:-[0-9]+)?)\.iso`})
	got, err := src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "9.3-1" || got.URL != page+"#:~:text=9.3%2D1" {
		t.Fatalf("got %+v", got)
	}
}

func TestNoMatch(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"nightly","html_url":"x"}]`)
	}))
	src, _ := New(c, config.Source{Type: config.TypeGitHubRelease, Repo: "a/b", Pattern: config.DefaultPattern})
	if _, err := src.Latest(context.Background()); err != ErrNoMatch {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
}
