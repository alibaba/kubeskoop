package nettop

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func getAllPids() ([]int, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pidlist := []int{}
	for _, name := range names {
		pid, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			continue
		}
		pidlist = append(pidlist, int(pid))
	}

	return pidlist, nil
}

func getNsInumByPid(pid int) (int, error) {
	d, err := os.Open(fmt.Sprintf("/proc/%d/ns", pid))
	if err != nil {
		return 0, err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return 0, fmt.Errorf("failed to read contents of ns dir: %w", err)
	}

	for _, name := range names {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/%s", pid, name))
		if err != nil {
			return 0, err
		}

		fields := strings.SplitN(target, ":", 2)
		if len(fields) != 2 {
			return 0, fmt.Errorf("failed to parse namespace type and inode from %q", target)
		}

		if fields[0] == "net" {
			inode, err := strconv.ParseUint(strings.Trim(fields[1], "[]"), 10, 32)
			if err != nil {
				return 0, fmt.Errorf("failed to parse inode from %q: %w", fields[1], err)
			}

			return int(inode), nil
		}

	}

	return 0, fmt.Errorf("net namespace of %d not found", pid)
}

func findNsfsMountpoint() (mounts []string, err error) {
	output, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}

	// /proc/mounts has 6 fields per line, one mount per line, e.g.
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, " ")
		if len(parts) == 6 {
			switch parts[2] {
			case "nsfs":
				mounts = append(mounts, parts[1])
			}
		}
	}

	return
}

func getNsInumByNsfsMountPoint(file string) (int, error) {
	fileinfo, err := os.Stat(file)
	if os.IsNotExist(err) || err != nil {
		return 0, err
	}
	if fileinfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(file)
		if err != nil {
			return 0, err
		}

		fields := strings.SplitN(target, ":", 2)
		if len(fields) != 2 {
			return 0, fmt.Errorf("failed to parse namespace type and inode from %q", target)
		}

		if fields[0] == "net" {
			inode, err := strconv.ParseUint(strings.Trim(fields[1], "[]"), 10, 32)
			if err != nil {
				return 0, fmt.Errorf("failed to parse inode from %q: %w", fields[1], err)
			}

			return int(inode), nil
		}
	} else {
		stat, ok := fileinfo.Sys().(*syscall.Stat_t)
		if !ok {
			return 0, fmt.Errorf("cannot parse file stat %s", file)
		}

		if stat.Ino != 0 {
			return int(stat.Ino), nil
		}
	}

	return 0, fmt.Errorf("cannot find valid inode of %s", file)
}
