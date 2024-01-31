package tracesoftirq

import (
	"testing"
)

func TestSoftirqTypesBits(t *testing.T) {
	bits := softirqTypesBits([]string{"net_rx", "net_tx", "rcu"})
	if bits != 0b1000001100 {
		t.Fatalf("softirqTypesBits not correct, 0b1000001100 != %x", bits)
	}
}

func TestTracesoftirq(t *testing.T) {
	result := enabledIrqTypes(0b1000001000)
	if len(result) != 2 || result[0] != "net_rx" || result[1] != "rcu" {
		t.Fatalf("enabledIrqTypes not correct, [net_rx,rcu] != %v", result)
	}
}
