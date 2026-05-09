package nettop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

var (
	cgroupRoot   = ""
	cgroupV2Mode = false
)

func init() {
	root, err := lookupCgroupRoot()
	if err != nil {
		log.Errorf("failed lookup cgroup root: %v", err)
		return
	}
	cgroupRoot = root
	cgroupV2Mode = isCgroupV2()
}

// isCgroupV2 checks if system is using cgroup v2
func isCgroupV2() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

func lookupCgroupRoot() (string, error) {
	// TODO lookup from /proc/mount
	return "/sys/fs/cgroup", nil
}

func tasksInsidePodCgroup(path string, absolutePath bool) []int {
	//TODO watch file changes by inotify
	if cgroupRoot == "" || path == "" {
		return nil
	}
	base := path
	if !absolutePath {
		if cgroupV2Mode {
			base = filepath.Join(cgroupRoot, path)
		} else {
			base = filepath.Join(cgroupRoot, "memory", path)
		}
	}

	// Determine the tasks file name based on cgroup version
	tasksFileName := "tasks"
	if cgroupV2Mode {
		tasksFileName = "cgroup.threads"
	}

	m := make(map[int]struct{})
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, "/"+tasksFileName) {
			tasks, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed read cgroup tasks %s: %w", path, err)
			}
			if err := parseTaskPIDs(tasks, m); err != nil {
				return fmt.Errorf("invalid tasks pid format in %s : %w", path, err)
			}
		}
		return nil
	})

	if err != nil {
		log.Errorf("failed list tasks: %v", err)
	}

	ret := make([]int, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}

func parseTaskPIDs(data []byte, pids map[int]struct{}) error {
	pid := 0
	hasDigit := false
	for _, b := range data {
		switch {
		case b >= '0' && b <= '9':
			hasDigit = true
			pid = pid*10 + int(b-'0')
		case b == '\n' || b == '\r' || b == '\t' || b == ' ':
			if hasDigit {
				pids[pid] = struct{}{}
				pid = 0
				hasDigit = false
			}
		default:
			return fmt.Errorf("unexpected byte %q", b)
		}
	}
	if hasDigit {
		pids[pid] = struct{}{}
	}
	return nil
}
