package procsoftnet

import (
	"bufio"
	"context"
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"golang.org/x/exp/slog"
)

const (
	SNProcessed = "processed"
	SNDropped   = "dropped"

	probeName = "softnet"
)

var (
	softnetMetrics = []string{SNProcessed, SNDropped}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, softNetProbeCreator)
}

func softNetProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcSoftnet{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, softnetMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcSoftnet struct {
}

func (s *ProcSoftnet) Start(_ context.Context) error {
	return nil
}

func (s *ProcSoftnet) Stop(_ context.Context) error {
	return nil
}

func (s *ProcSoftnet) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllUniqueNetNSEntity()
	if len(ets) == 0 {
		slog.Info("collect", "mod", probeName, "ignore", "no entity found")
	}
	return collect(ets)
}

func collect(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)

	for _, m := range softnetMetrics {
		resMap[m] = map[uint32]uint64{}
	}

	for _, ns := range nslist {
		stat, err := getSoftnetStatByPid(uint32(ns.GetPid()))
		if err != nil {
			continue
		}
		for _, m := range softnetMetrics {
			resMap[m][uint32(ns.GetNetns())] = stat[m]
		}
	}
	return resMap, nil
}

type SoftnetStat struct {
	// Number of processed packets.
	Processed uint32
	// Number of dropped packets.
	Dropped uint32
	// Number of times processing packets ran out of quota.
	TimeSqueezed uint32
}

func getSoftnetStatByPid(pid uint32) (map[string]uint64, error) {
	snfile := fmt.Sprintf("/proc/%d/net/softnet_stat", pid)
	if _, err := os.Stat(snfile); os.IsNotExist(err) {
		return nil, err
	}
	sns, err := parseSoftnet(snfile)
	if err != nil {
		return nil, err
	}

	res := map[string]uint64{}
	for _, ns := range sns {
		res[SNProcessed] += uint64(ns.Processed)
		res[SNDropped] += uint64(ns.Dropped)
	}

	return res, nil
}

func parseSoftnet(file string) ([]SoftnetStat, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := io.LimitReader(f, 1024*512)

	var minColumns = 9

	s := bufio.NewScanner(reader)

	var stats []SoftnetStat
	for s.Scan() {
		columns := strings.Fields(s.Text())
		width := len(columns)

		if width < minColumns {
			return nil, fmt.Errorf("%d columns were detected, but at least %d were expected", width, minColumns)
		}

		// We only parse the first three columns at the moment.
		us, err := parseHexUint32s(columns[0:3])
		if err != nil {
			return nil, err
		}

		stats = append(stats, SoftnetStat{
			Processed:    us[0],
			Dropped:      us[1],
			TimeSqueezed: us[2],
		})
	}

	return stats, nil
}

func parseHexUint32s(ss []string) ([]uint32, error) {
	us := make([]uint32, 0, len(ss))
	for _, s := range ss {
		u, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return nil, err
		}

		us = append(us, uint32(u))
	}

	return us, nil
}
