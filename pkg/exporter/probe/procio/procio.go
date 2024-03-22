package procio

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/procfs"
)

const (
	IOReadSyscall  = "readsyscall"
	IOWriteSyscall = "writesyscall"
	IOReadBytes    = "readbytes"
	IOWriteBytes   = "writebytes"

	probeName = "io" // nolint
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, ioProbeCreator)
}

func ioProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcIO{}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: probe.StandardMetricsLabels,
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: IOReadSyscall, ValueType: prometheus.CounterValue},
			{Name: IOWriteSyscall, ValueType: prometheus.CounterValue},
			{Name: IOReadBytes, ValueType: prometheus.CounterValue},
			{Name: IOWriteBytes, ValueType: prometheus.CounterValue},
		},
	}
	metrics := probe.NewBatchMetrics(opts, p.collectOnce)

	return probe.NewMetricsProbe(probeName, p, metrics), nil
}

type ProcIO struct {
}

func (s *ProcIO) Start(_ context.Context) error {
	return nil
}

func (s *ProcIO) Stop(_ context.Context) error {
	return nil
}

func (s *ProcIO) collectOnce(emit probe.Emit) error {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		log.Infof("procio: no entity found")
	}
	for _, entity := range ets {
		collectProcessIO(entity, emit)
	}
	return nil
}

func collectProcessIO(entity *nettop.Entity, emit probe.Emit) {
	var (
		readSyscall  uint64
		writeSyscall uint64
		readBytes    uint64
		writeBytes   uint64
	)
	for _, pid := range entity.GetPids() {
		iodata, err := getProcessIOStat(pid)
		if err != nil {
			log.Warningf("probe %s: failed get process io data: %v", probeName, err)
			continue
		}

		readSyscall += iodata.SyscR
		writeSyscall += iodata.SyscW
		readBytes += iodata.ReadBytes
		writeBytes += iodata.WriteBytes
	}
	labels := probe.BuildStandardMetricsLabelValues(entity)
	emit(IOReadSyscall, labels, float64(readSyscall))
	emit(IOWriteSyscall, labels, float64(writeSyscall))
	emit(IOReadBytes, labels, float64(readBytes))
	emit(IOWriteBytes, labels, float64(writeBytes))
}

// IO creates a new ProcIO instance from a given Proc instance.
func getProcessIOStat(pid int) (procfs.ProcIO, error) {
	pio := procfs.ProcIO{}

	data, err := readFileNoStat(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return pio, err
	}

	ioFormat := "rchar: %d\nwchar: %d\nsyscr: %d\nsyscw: %d\n" +
		"read_bytes: %d\nwrite_bytes: %d\n" +
		"cancelled_write_bytes: %d\n"

	_, err = fmt.Sscanf(string(data), ioFormat, &pio.RChar, &pio.WChar, &pio.SyscR,
		&pio.SyscW, &pio.ReadBytes, &pio.WriteBytes, &pio.CancelledWriteBytes)

	return pio, err
}

func readFileNoStat(filename string) ([]byte, error) {
	const maxBufferSize = 1024 * 512

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := io.LimitReader(f, maxBufferSize)
	return io.ReadAll(reader)
}
