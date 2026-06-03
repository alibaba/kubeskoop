package taskagent

import "testing"

func TestValidateCaptureFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		wantErr bool
	}{
		{"empty filter", "", false},
		{"simple host filter", "host 10.0.0.1", false},
		{"port filter", "port 80", false},
		{"complex filter", "tcp port 80 and host 10.0.0.1", false},
		{"filter with parens", "(tcp or udp) and port 80", false},
		{"filter with comparison", "len > 100", false},
		{"bpf arithmetic", "ip[0:1] & 0xf * 4 + 8", false},
		{"bpf cidr", "net 10.0.0.0/24", false},
		{"bpf portrange", "portrange 80-443", false},
		{"bpf tcp flags", "tcp[tcpflags] & tcp-syn != 0", false},
		{"injection semicolon", "; rm -rf /", true},
		{"injection backtick", "`id`", true},
		{"injection dollar", "$(whoami)", true},
		{"injection newline", "host 10.0.0.1\nid", true},
		{"injection pipe to shell", "x | sh", false},
		{"flag injection -z", "-z /bin/sh", true},
		{"flag injection -r", "-r /etc/shadow", true},
		{"flag injection -w", "-w /tmp/evil", true},
		{"flag injection -G", "-G 1", true},
		{"flag injection mixed", "-z /tmp/x -G 1 port 80", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCaptureFilter(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCaptureFilter(%q) error = %v, wantErr %v", tt.filter, err, tt.wantErr)
			}
		})
	}
}

func TestBuildTcpdumpArgs(t *testing.T) {
	args := buildTcpdumpArgs("/tmp/test.pcap", "tcp port 80")
	expected := []string{"-i", "any", "-C", "100", "-w", "/tmp/test.pcap", "--", "tcp", "port", "80"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(expected), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, a, expected[i])
		}
	}
}

func TestBuildTcpdumpArgsEmpty(t *testing.T) {
	args := buildTcpdumpArgs("/tmp/test.pcap", "")
	expected := []string{"-i", "any", "-C", "100", "-w", "/tmp/test.pcap"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(expected), args)
	}
}
