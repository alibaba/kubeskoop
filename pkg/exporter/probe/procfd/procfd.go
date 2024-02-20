package procfd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

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
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, fdProbeCreator)
}

func fdProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcFD{}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: probe.StandardMetricsLabels,
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: OpenFD, ValueType: prometheus.GaugeValue},
			{Name: OpenSocket, ValueType: prometheus.GaugeValue},
		},
	}
	metrics := probe.NewBatchMetrics(opts, p.collectOnce)

	return probe.NewMetricsProbe(probeName, p, metrics), nil
}

type ProcFD struct {
}

func (s *ProcFD) Start(_ context.Context) error {
	return nil
}

func (s *ProcFD) Stop(_ context.Context) error {
	return nil
}

func (s *ProcFD) collectOnce(emit probe.Emit) error {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		log.Warningf("procfd: no entity found")
		return nil
	}
	for _, entity := range ets {
		collectProcessFd(entity, emit)
		emit(OpenFD, probe.BuildStandardMetricsLabelValues(entity), 0)
	}
	return nil
}

func collectProcessFd(entity *nettop.Entity, emit probe.Emit) {
	var (
		fdCount   int
		sockCount int
	)
	for _, idx := range entity.GetPids() {
		procfds, err := getProcessFdStat(idx)
		if err != nil {
			log.Warningf("failed open proc fd for pod %s: %v", entity, err)
			return
		}

		for fd := range procfds {
			fdCount++
			if strings.HasPrefix(fd, "socket:") {
				sockCount++
			}
		}
	}
	labels := probe.BuildStandardMetricsLabelValues(entity)
	emit(OpenFD, labels, float64(fdCount))
	emit(OpenSocket, labels, float64(sockCount))
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
		return nil, fmt.Errorf("could not read %s: %w", fdpath, err)
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
