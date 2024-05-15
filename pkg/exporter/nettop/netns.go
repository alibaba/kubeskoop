package nettop

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func getNsInumByPid(pid int) (int, error) {
	d, err := os.Open(fmt.Sprintf("/proc/%d/ns", pid))
	if err != nil {
		return 0, err
	}
	defer d.Close()

	target, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", pid))
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

	return 0, fmt.Errorf("net namespace of %d not found", pid)
}
