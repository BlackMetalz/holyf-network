package collector

import "testing"

func TestParseInterfaceSpeedMbpsValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		want   float64
		wantOK bool
	}{
		{name: "plain", raw: "1000\n", want: 1000, wantOK: true},
		{name: "whitespace", raw: " 25000 ", want: 25000, wantOK: true},
		{name: "unknown negative", raw: "-1", want: 0, wantOK: false},
		{name: "zero", raw: "0", want: 0, wantOK: false},
		{name: "invalid", raw: "n/a", want: 0, wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseInterfaceSpeedMbpsValue(tc.raw)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("parseInterfaceSpeedMbpsValue(%q): got=(%v,%v) want=(%v,%v)", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
