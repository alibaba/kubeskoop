package taskagent

import "testing"

func TestGetLatency(t *testing.T) {
	pingStr := `PING 8.8.8.8 (8.8.8.8): 56 data bytes

--- 8.8.8.8 ping statistics ---
100 packets transmitted, 100 packets received, 0% packet loss
round-trip min/avg/max = 43.689/43.720/43.809 ms`
	min, avg, max, err := getLatency(pingStr)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if min != 43.689 || avg != 43.720 || max != 43.809 {
		t.Fatalf("min/avg/max is not correct")
	}
	t.Logf("min/avg/max is %v, %v, %v", min, avg, max)
}
