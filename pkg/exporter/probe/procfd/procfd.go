package procfd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	log "k8s.io/klog/v2"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
)

const (
	probeName = "fd"
)

var (
	OpenFD     = "openfd"
	OpenSocket = "opensocket"
	FDMetrics  = []string{OpenFD, OpenSocket}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, fdProbeCreator)
}

func fdProbeCreator(_ map[string]interface{}) (probe.MetricsProbe, error) {
	p := &ProcFD{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, FDMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcFD struct {
}

func (s *ProcFD) Start(_ context.Context) error {
	return nil
}

func (s *ProcFD) Stop(_ context.Context) error {
	return nil
}

func (s *ProcFD) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		log.Warningf("procfd: no entity found")
		return map[string]map[uint32]uint64{}, nil
	}
	return getAllProcessFd(ets)
}

func getAllProcessFd(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)

	for _, m := range FDMetrics {
		resMap[m] = make(map[uint32]uint64)
	}

	for _, nslogic := range nslist {
		nsprocfd := map[string]struct{}{}
		nsprocfsock := map[string]struct{}{}
		for _, idx := range nslogic.GetPids() {
			procfds, err := getProcessFdStat(idx)
			if err != nil && !os.IsNotExist(err) {
				return resMap, err
			}

			for fd := range procfds {
				nsprocfd[fd] = struct{}{}
				if strings.HasPrefix(fd, "socket:") {
					nsprocfsock[fd] = struct{}{}
				}
			}
		}
		resMap[OpenFD][uint32(nslogic.GetNetns())] = uint64(len(nsprocfd))
		resMap[OpenSocket][uint32(nslogic.GetNetns())] = uint64(len(nsprocfsock))
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
