package version

import (
	"strings"
	"unicode"
)

func Normalize(tag string) string {
	i := strings.IndexFunc(tag, unicode.IsDigit)
	if i < 0 {
		return ""
	}
	return tag[i:]
}

func Compare(a, b string) int {
	for {
		as, ar, aNum := segment(a)
		bs, br, bNum := segment(b)
		switch {
		case as == "" && bs == "":
			return 0
		case as == "":
			if isPrerelease(b) {
				return 1
			}
			return -1
		case bs == "":
			if isPrerelease(a) {
				return -1
			}
			return 1
		}
		var c int
		if aNum && bNum {
			c = compareNumeric(as, bs)
		} else {
			c = strings.Compare(as, bs)
		}
		if c != 0 {
			return c
		}
		a, b = ar, br
	}
}

func segment(s string) (seg, rest string, numeric bool) {
	if s == "" {
		return "", "", false
	}
	numeric = isDigit(s[0])
	i := 1
	for i < len(s) && isDigit(s[i]) == numeric {
		i++
	}
	return s[:i], s[i:], numeric
}

// isPrerelease reports whether the remainder of a version after a shared
// prefix marks a pre-release ("-rc1", ".beta", "a1") rather than a further
// release component ("-1", ".2"). So 1.10.0-rc1 < 1.10.0 < 1.10.0-1.
func isPrerelease(rest string) bool {
	rest = strings.TrimLeft(rest, "-._~")
	return rest != "" && !isDigit(rest[0]) && rest[0] != '+'
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func compareNumeric(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}
