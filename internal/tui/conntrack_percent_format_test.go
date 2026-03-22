package tui

import "testing"

func TestFormatConntrackPercentShort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   float64
		expects string
	}{
		{name: "zero", input: 0, expects: "0%"},
		{name: "tiny", input: 0.0007, expects: "<0.1%"},
		{name: "sub one", input: 0.4, expects: "0.4%"},
		{name: "normal", input: 2.4, expects: "2%"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatConntrackPercentShort(tc.input); got != tc.expects {
				t.Fatalf("formatConntrackPercentShort(%v): got=%q want=%q", tc.input, got, tc.expects)
			}
		})
	}
}

func TestFormatConntrackPercentDetailed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   float64
		expects string
	}{
		{name: "zero", input: 0, expects: "0.0%"},
		{name: "tiny", input: 0.0007, expects: "<0.1%"},
		{name: "normal", input: 2.4, expects: "2.4%"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatConntrackPercentDetailed(tc.input); got != tc.expects {
				t.Fatalf("formatConntrackPercentDetailed(%v): got=%q want=%q", tc.input, got, tc.expects)
			}
		})
	}
}
