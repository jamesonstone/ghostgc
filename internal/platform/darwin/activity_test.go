//go:build darwin

package darwin

import "testing"

func TestWithinRepositoryUsesPathBoundaries(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/repo", want: true},
		{path: "/repo/file.go", want: true},
		{path: "/repo/sub/file.go", want: true},
		{path: "/repo-other/file.go", want: false},
		{path: "/outside/file.go", want: false},
	}
	for _, tt := range tests {
		if got := withinRepository("/repo", tt.path); got != tt.want {
			t.Errorf("withinRepository(/repo, %q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}
