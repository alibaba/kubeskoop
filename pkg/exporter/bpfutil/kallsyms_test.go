package bpfutil

import (
	"math/rand"
	"testing"
	"time"
)

func TestGetSymPtFromBpfLocation(t *testing.T) {
	now := time.Now()
	for i := 0; i < len(kallsyms)-1; i++ {
		offset := rand.Intn(100)
		_, err := GetSymPtFromBpfLocation(kallsyms[i].start + uint64(offset))
		if err != nil {
			t.Fatal("failed to get symbol point", kallsyms[i], err)
		}
	}
	t1 := time.Now()
	t.Logf("time cost: %v\n", t1.Sub(now))
}
