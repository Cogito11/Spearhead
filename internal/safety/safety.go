// Package safety scans outgoing text for things that look like credentials.
package safety

import (
	"math"
	"regexp"
	"strings"
)

type Finding struct {
	Rule   string `json:"rule"`
	Sample string `json:"sample"` // redacted
}

var rules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"AWS access key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"JWT", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"credentials in URL", regexp.MustCompile(`://[^/\s:@]+:[^/\s@]+@`)},
	{"password/secret assignment", regexp.MustCompile(`(?i)\b(pass(word|wd)?|pwd|secret|api[_-]?key|token)\b\s*[:=]\s*\S{4,}`)},
	{"\"password is ...\"", regexp.MustCompile(`(?i)\bpass(word|phrase)\s+is\s+\S+`)},
}

var tokenRe = regexp.MustCompile(`[A-Za-z0-9+/=_-]{24,}`)

func redact(s string) string {
	if len(s) <= 6 {
		return "***"
	}
	return s[:3] + strings.Repeat("*", 5) + " (" + itoa(len(s)) + " chars)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func entropy(s string) float64 {
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	var h float64
	n := float64(len(s))
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// Scan returns findings for text that looks like it contains a credential.
func Scan(text string) []Finding {
	var out []Finding
	for _, r := range rules {
		for _, m := range r.re.FindAllString(text, 3) {
			out = append(out, Finding{r.name, redact(m)})
		}
	}
	for _, tok := range tokenRe.FindAllString(text, -1) {
		hasDigit := strings.ContainsAny(tok, "0123456789")
		hasAlpha := strings.ContainsAny(tok, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
		if hasDigit && hasAlpha && entropy(tok) >= 4.0 {
			out = append(out, Finding{"high-entropy token", redact(tok)})
		}
	}
	return out
}
