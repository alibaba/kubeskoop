package procio

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/procfs"
)

func writeTaskFile(t *testing.T, root string, tid int, name, content string) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(tid))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectProcessIOStatsDeduplicatesThreadGroups(t *testing.T) {
	root := t.TempDir()
	for _, tid := range []int{100, 101, 102} {
		writeTaskFile(t, root, tid, "status", "Name:\tworker\nTgid:\t100\n")
		writeTaskFile(t, root, tid, "io", "syscr: 10\nsyscw: 20\nread_bytes: 30\nwrite_bytes: 40\n")
	}
	writeTaskFile(t, root, 200, "status", "Tgid:\t200\n")
	writeTaskFile(t, root, 200, "io", "syscr: 1\nsyscw: 2\nread_bytes: 3\nwrite_bytes: 4\n")
	want := procfs.ProcIO{SyscR: 11, SyscW: 22, ReadBytes: 33, WriteBytes: 44}
	// A non-leader can be encountered first; duplicate task IDs and later
	// scrapes must neither double-count nor suppress the process's counters.
	for i := 0; i < 2; i++ {
		got := collectProcessIOStats(root, []int{101, 100, 102, 101, 200})
		if got != want {
			t.Fatalf("scrape %d: got %+v, want %+v", i, got, want)
		}
	}
}

func TestCollectProcessIOStatsExitingTasks(t *testing.T) {
	root := t.TempDir()
	// Task 100 vanished entirely. Task 101 exited between status and io.
	// A surviving non-leader must still account for the process.
	writeTaskFile(t, root, 101, "status", "Tgid:\t100\n")
	writeTaskFile(t, root, 102, "status", "Tgid:\t100\n")
	writeTaskFile(t, root, 102, "io", "syscr: 7\nwrite_bytes: 42\n")
	got := collectProcessIOStats(root, []int{100, 101, 102})
	want := procfs.ProcIO{SyscR: 7, WriteBytes: 42}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGetThreadGroupID(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		want         int
	}{
		{"valid", "Name:\tworker\nTgid:\t123\nPid:\t124\n", 123},
		{"no trailing newline", "Tgid:\t123", 123},
		{"missing", "Pid:\t124\n", 0},
		{"malformed", "Tgid:\tnot-a-pid\n", 0},
		{"zero", "Tgid:\t0\n", 0},
		{"negative", "Tgid:\t-1\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTaskFile(t, root, 124, "status", tc.status)
			got, err := getThreadGroupID(root, 124)
			if tc.want == 0 {
				if err == nil {
					t.Fatal("expected invalid TGID error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}
