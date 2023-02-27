package procnetdev

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/prometheus/procfs"
	"golang.org/x/exp/slog"
)

const (
	MODULE_NAME = "procnetdev" // nolint

	RxBytes   = "RxBytes"
	RxErrors  = "RxErrors"
	TxBytes   = "TxBytes"
	TxErrors  = "TxErrors"
	RxPackets = "RxPackets"
	RxDropped = "RxDropped"
	TxPackets = "TxPackets"
	TxDropped = "TxDropped"
)

var (
	once  = sync.Once{}
	probe *ProcNetdev

	NetdevMetrics = []string{RxBytes, RxErrors, TxBytes, TxErrors, RxPackets, RxDropped, TxPackets, TxDropped}
)

type ProcNetdev struct {
}

func GetProbe() *ProcNetdev {
	once.Do(func() {
		probe = &ProcNetdev{}
	})
	return probe
}

func (s *ProcNetdev) Close() error {
	return nil
}

func (s *ProcNetdev) Start(_ context.Context) {
}

func (s *ProcNetdev) Ready() bool {
	// determine by if default snmp file was ready
	if _, err := os.Stat("/proc/net/snmp"); os.IsNotExist(err) {
		return false
	}
	return true
}

func (s *ProcNetdev) Name() string {
	return MODULE_NAME
}

func (s *ProcNetdev) GetMetricNames() []string {
	res := []string{}
	for _, m := range NetdevMetrics {
		res = append(res, metricUniqueID("netdev", m))
	}
	return res
}

func (s *ProcNetdev) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		slog.Ctx(ctx).Info("collect", "mod", MODULE_NAME, "ignore", "no entity found")
	}
	return collect(ctx, ets)
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

func collect(ctx context.Context, nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, mname := range NetdevMetrics {
		resMap[metricUniqueID("netdev", mname)] = map[uint32]uint64{}
	}
	netdev := getAllNetdev(nslist)

	for nsid := range netdev {
		if len(netdev[nsid]) == 0 {
			continue
		}

		for _, mname := range NetdevMetrics {
			resMap[metricUniqueID("netdev", mname)][nsid] = 0
		}

		for devname, devstat := range netdev[nsid] {
			if devname != "lo" {
				resMap[metricUniqueID("netdev", RxBytes)][nsid] += devstat.RxBytes
				resMap[metricUniqueID("netdev", RxErrors)][nsid] += devstat.RxErrors
				resMap[metricUniqueID("netdev", TxBytes)][nsid] += devstat.TxBytes
				resMap[metricUniqueID("netdev", TxErrors)][nsid] += devstat.TxErrors
				resMap[metricUniqueID("netdev", RxPackets)][nsid] += devstat.RxPackets
				resMap[metricUniqueID("netdev", TxPackets)][nsid] += devstat.TxPackets
				resMap[metricUniqueID("netdev", RxDropped)][nsid] += devstat.RxDropped
				resMap[metricUniqueID("netdev", TxDropped)][nsid] += devstat.TxDropped
			}
		}
	}
	return resMap, nil
}

func getAllNetdev(nslist []*nettop.Entity) map[uint32]procfs.NetDev {
	allnetdevs := map[uint32]procfs.NetDev{}

	for idx := range nslist {
		netdev, err := getNetdevByPid(uint32(nslist[idx].GetPid()))
		if err != nil {
			continue
		}

		allnetdevs[uint32(nslist[idx].GetNetns())] = netdev
	}

	return allnetdevs
}

func getNetdevByPid(pid uint32) (procfs.NetDev, error) {

	fs, err := procfs.NewProc(int(pid))
	if err != nil {
		return nil, err
	}
	netdev, err := fs.NetDev()
	if err != nil {
		return nil, err
	}

	return netdev, nil
}
