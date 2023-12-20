package nettop

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	cgroupRoot = ""
)

func init() {
	root, err := lookupCgroupRoot()
	if err != nil {
		log.Errorf("failed lookup cgroup root: %v", err)
		return
	}
	cgroupRoot = root
}

func lookupCgroupRoot() (string, error) {
	//TODO lookup from /proc/mount
	return "/sys/fs/cgroup", nil
}

func tasksInsidePodCgroup(path string) []int {
	//TODO watch file changes by inotify
	if cgroupRoot == "" || path == "" {
		return nil
	}
	base := filepath.Join(cgroupRoot, "memory", path)
	m := make(map[int]int)
	err := filepath.Walk(base, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, "/tasks") {
			tasks, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed read cgroup tasks %s: %w", path, err)
			}
			for _, s := range strings.Split(string(tasks), "\n") {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				i, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("invalid tasks pid format in %s : %w", path, err)
				}
				m[i] = 1
			}
		}
		return nil
	})

	if err != nil {
		log.Errorf("failed list tasks: %v", err)
	}

	var ret []int
	for k, _ := range m {
		ret = append(ret, k)
	}
	return ret
}
