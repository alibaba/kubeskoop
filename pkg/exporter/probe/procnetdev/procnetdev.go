package procnetdev

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/procfs"
	log "github.com/sirupsen/logrus"
)

const (
	probeName = "netdev" // nolint

	RxBytes   = "rxbytes"
	RxErrors  = "rxerrors"
	TxBytes   = "txbytes"
	TxErrors  = "txerrors"
	RxPackets = "rxpackets"
	RxDropped = "rxdropped"
	TxPackets = "txpackets"
	TxDropped = "txdropped"
)

var (
	NetdevMetrics = []string{RxBytes, RxErrors, TxBytes, TxErrors, RxPackets, RxDropped, TxPackets, TxDropped}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, netdevProbeCreator)
}

func netdevProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcNetdev{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, NetdevMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcNetdev struct {
}

func (s *ProcNetdev) Start(_ context.Context) error {
	return nil
}

func (s *ProcNetdev) Stop(_ context.Context) error {
	return nil
}

func (s *ProcNetdev) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllUniqueNetnsEntity()
	if len(ets) == 0 {
		log.Errorf("%s error, no entity found", probeName)
	}
	return collect(ets)
}

func collect(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, m := range NetdevMetrics {
		resMap[m] = make(map[uint32]uint64)
	}

	netdev := getAllNetdev(nslist)

	for nsid := range netdev {
		if len(netdev[nsid]) == 0 {
			continue
		}

		for devname, devstat := range netdev[nsid] {
			if devname != "lo" {
				resMap[RxBytes][nsid] += devstat.RxBytes
				resMap[RxErrors][nsid] += devstat.RxErrors
				resMap[TxBytes][nsid] += devstat.TxBytes
				resMap[TxErrors][nsid] += devstat.TxErrors
				resMap[RxPackets][nsid] += devstat.RxPackets
				resMap[TxPackets][nsid] += devstat.TxPackets
				resMap[RxDropped][nsid] += devstat.RxDropped
				resMap[TxDropped][nsid] += devstat.TxDropped
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
