package history

import "testing"

func TestShouldUseSystemDefaultPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		goos string
		euid int
		want bool
	}{
		{name: "linux root", goos: "linux", euid: 0, want: true},
		{name: "linux non-root", goos: "linux", euid: 1000, want: false},
		{name: "darwin root", goos: "darwin", euid: 0, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldUseSystemDefaultPaths(tc.goos, tc.euid)
			if got != tc.want {
				t.Fatalf("shouldUseSystemDefaultPaths(%q,%d)=%t want=%t", tc.goos, tc.euid, got, tc.want)
			}
		})
	}
}
