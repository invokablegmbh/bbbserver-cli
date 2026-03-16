package config

import "testing"

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "abcd", want: "****"},
		{in: "12345678", want: "****5678"},
	}

	for _, tc := range tests {
		if got := MaskAPIKey(tc.in); got != tc.want {
			t.Fatalf("MaskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
