package procfd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"golang.org/x/exp/slog"
)

const (
	ModuleName = "Procfd" // nolint
)

var (
	probe = &ProcFD{}

	FDMetrics = []string{"OpenFd", "OpenSocket"}
)

type ProcFD struct {
}

func GetProbe() *ProcFD {
	return probe
}

func (s *ProcFD) Close() error {
	return nil
}

func (s *ProcFD) Start(_ context.Context) {
}

func (s *ProcFD) Ready() bool {
	return true
}

func (s *ProcFD) Name() string {
	return ModuleName
}

func (s *ProcFD) GetMetricNames() []string {
	res := []string{}
	for _, m := range FDMetrics {
		res = append(res, metricUniqueID("fd", m))
	}
	return res
}

func (s *ProcFD) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		slog.Ctx(ctx).Info("collect", "mod", ModuleName, "ignore", "no entity found")
	}
	return getAllProcessFd(ets)
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

func getAllProcessFd(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, metricname := range FDMetrics {
		resMap[metricUniqueID("fd", metricname)] = map[uint32]uint64{}
	}
	for _, nslogic := range nslist {
		nsprocfd := map[string]struct{}{}
		nsprocfsock := map[string]struct{}{}
		for _, idx := range nslogic.GetPids() {
			procfds, err := getProcessFdStat(idx)
			if err != nil {
				return resMap, err
			}
			for fd := range procfds {
				nsprocfd[fd] = struct{}{}
				if strings.HasPrefix(fd, "socket:") {
					nsprocfsock[fd] = struct{}{}
				}
			}
		}
		resMap[metricUniqueID("fd", "OpenFd")][uint32(nslogic.GetNetns())] = uint64(len(nsprocfd))
		resMap[metricUniqueID("fd", "OpenSocket")][uint32(nslogic.GetNetns())] = uint64(len(nsprocfsock))
	}
	return resMap, nil
}

func getProcessFdStat(pid int) (map[string]struct{}, error) {
	fdpath := fmt.Sprintf("/proc/%d/fd", pid)
	d, err := os.Open(fdpath)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %w", d.Name(), err)
	}

	fds := map[string]struct{}{}
	for _, name := range names {
		fdlink := fmt.Sprintf("%s/%s", fdpath, name)
		info, err := os.Readlink(fdlink)
		if os.IsNotExist(err) {
			continue
		}
		fds[info] = struct{}{}
	}

	return fds, nil
}
