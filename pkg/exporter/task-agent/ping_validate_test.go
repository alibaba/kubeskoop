package taskagent

import (
	"net"
	"testing"
)

func TestPingDestinationValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"valid ipv4", "10.0.0.1", true},
		{"valid ipv6", "::1", true},
		{"valid ipv4 full", "192.168.1.100", true},
		{"injection semicolon", "; rm -rf /", false},
		{"injection backtick", "`id`", false},
		{"injection dollar", "$(whoami)", false},
		{"hostname", "example.com", false},
		{"empty", "", false},
		{"spaces", "10.0.0.1 ; id", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := net.ParseIP(tt.input) != nil
			if result != tt.valid {
				t.Errorf("net.ParseIP(%q) valid = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}
