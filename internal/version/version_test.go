package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.37.1", "1.37.1", 0},
		{"1.37.1", "1.37.10", -1},
		{"1.9.0", "1.10.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"9.2-1", "9.3-1", -1},
		{"9.3-2", "9.3-1", 1},
		{"26.9.13.15-stable", "26.10.1.1-stable", -1},
		{"1.2", "1.2.1", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"v1.37.1":        "1.37.1",
		"release-3.9.6":  "3.9.6",
		"docker-v29.0.0": "29.0.0",
		"2.4.3":          "2.4.3",
		"latest":         "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
