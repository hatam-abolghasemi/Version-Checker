package notify

import (
	"testing"

	"github.com/hatam-abolghasemi/Version-Checker/internal/config"
	"github.com/hatam-abolghasemi/Version-Checker/internal/source"
)

func TestDefaultTemplate(t *testing.T) {
	r, err := NewRenderer("")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := r.Render(Report{Updates: []Update{
		{Tool: config.Tool{Name: "kubernetes"}, Old: "1.37.0", New: source.Release{Version: "1.37.1", URL: "https://x/v1.37.1"}},
		{Tool: config.Tool{Name: "argocd", DisplayName: "Argo CD"}, Old: "3.5.3", New: source.Release{Version: "3.5.4", URL: "https://y?a=1&b=2"}},
	}})
	want := "🆕 <b>New versions</b>\n\n<b>kubernetes</b>: 1.37.0 → <a href=\"https://x/v1.37.1\">1.37.1</a>\n<b>Argo CD</b>: 3.5.3 → <a href=\"https://y?a=1&amp;b=2\">3.5.4</a>"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	got, _ = r.Render(Report{
		Failures:  []Failure{{Tool: config.Tool{Name: "haproxy"}, Error: "GET x: 404 <Not Found>"}},
		Recovered: []config.Tool{{Name: "a"}, {Name: "b"}},
	})
	want = "⚠️ <b>Failing</b>\n\n<b>haproxy</b>: GET x: 404 &lt;Not Found&gt;\n\n✅ <b>Recovered</b>: a, b"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
