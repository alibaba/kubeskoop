package procsoftnet

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"golang.org/x/exp/slog"
)

const (
	SNProcessed = "Processed"
	SNDropped   = "Dropped"

	MODULE_NAME = "procsoftnet" // nolint
)

var (
	SoftnetMetrics = []string{SNProcessed, SNDropped}
	once           = sync.Once{}
	probe          *ProcSoftnet
)

type ProcSoftnet struct {
}

func GetProbe() *ProcSoftnet {
	once.Do(func() {
		probe = &ProcSoftnet{}
	})
	return probe
}

func (s *ProcSoftnet) Close() error {
	return nil
}

func (s *ProcSoftnet) Start(_ context.Context) {
}

func (s *ProcSoftnet) Ready() bool {
	// determine by if default snmp file was ready
	if _, err := os.Stat("/proc/net/snmp"); os.IsNotExist(err) {
		return false
	}
	return true
}

func (s *ProcSoftnet) Name() string {
	return MODULE_NAME
}

func (s *ProcSoftnet) GetMetricNames() []string {
	res := []string{}
	for _, m := range SoftnetMetrics {
		res = append(res, metricUniqueID("softnet", m))
	}
	return res
}

func (s *ProcSoftnet) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
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

	for idx := range SoftnetMetrics {
		resMap[metricUniqueID("softnet", SoftnetMetrics[idx])] = map[uint32]uint64{}
	}

	for idx := range nslist {
		stat, err := getSoftnetStatByPid(uint32(nslist[idx].GetPid()))
		if err != nil {
			continue
		}
		for indx := range SoftnetMetrics {
			resMap[metricUniqueID("softnet", SoftnetMetrics[indx])][uint32(nslist[idx].GetNetns())] = stat[SoftnetMetrics[indx]]
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
	for idx := range sns {
		res[SNProcessed] += uint64(sns[idx].Processed)
		res[SNDropped] += uint64(sns[idx].Dropped)
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
