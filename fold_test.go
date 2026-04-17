package xsearch

import "testing"

func TestFold_StripsCombiningMarks(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Moët", "Moet"},
		{"café", "cafe"},
		{"Curaçao", "Curacao"},
		{"naïve", "naive"},
		{"mœut", "moeut"},
		{"æther", "aether"},
		{"groß", "gross"},
		{"", ""},
		{"plain ascii", "plain ascii"},
		{"日本語", "日本語"}, // CJK no-op
		{"🍹", "🍹"},      // emoji no-op
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := Fold(tc.in)
			if got != tc.want {
				t.Errorf("Fold(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFold_Idempotent(t *testing.T) {
	for _, s := range []string{"Moët", "café", "naïve", "ascii", "日本語"} {
		once := Fold(s)
		twice := Fold(once)
		if once != twice {
			t.Errorf("Fold not idempotent for %q: once=%q twice=%q", s, once, twice)
		}
	}
}
