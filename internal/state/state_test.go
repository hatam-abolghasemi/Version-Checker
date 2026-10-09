package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLegacyAndCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"kubernetes":"1.37.1","coredns":{"version":"1.14.7","url":"u"}}`), 0o644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s["kubernetes"].Version != "1.37.1" || s["coredns"].Version != "1.14.7" || s["coredns"].URL != "u" {
		t.Fatalf("got %+v %+v", s["kubernetes"], s["coredns"])
	}
	s.Prune(map[string]bool{"coredns": true})
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	again, _ := Load(path)
	if len(again) != 1 || again["coredns"].URL != "u" {
		t.Fatalf("after save: %+v", again)
	}
}

func TestLoadMissing(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || len(s) != 0 {
		t.Fatalf("s=%v err=%v", s, err)
	}
}
