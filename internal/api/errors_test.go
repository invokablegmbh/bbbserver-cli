package api

import "testing"

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "validation", err: &ValidationError{Message: "bad input"}, want: ExitCodeUsage},
		{name: "network", err: &NetworkError{Message: "timeout"}, want: ExitCodeNetwork},
		{name: "auth", err: &APIError{Message: "nope", Status: 401}, want: ExitCodeAuth},
		{name: "not-found", err: &APIError{Message: "missing", Status: 404}, want: ExitCodeNotFound},
		{name: "server", err: &APIError{Message: "boom", Status: 500}, want: ExitCodeServer},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}
