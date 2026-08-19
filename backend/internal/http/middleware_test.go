package apiv1

import "testing"

func TestEquivalentLoopbackOrigins(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{"localhost and IPv4 loopback", "http://localhost:4200", "http://127.0.0.1:4200", true},
		{"loopback ports must match", "http://localhost:4201", "http://127.0.0.1:4200", false},
		{"schemes must match", "https://localhost:4200", "http://127.0.0.1:4200", false},
		{"non-loopback is rejected", "http://example.com:4200", "http://localhost:4200", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equivalentLoopbackOrigins(tt.left, tt.right); got != tt.want {
				t.Fatalf("equivalentLoopbackOrigins(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}
