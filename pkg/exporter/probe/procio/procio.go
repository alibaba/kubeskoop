package procio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

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

	maxProcIOBufferSize = 1024 * 512
)

var procIOBufferPool = sync.Pool{New: func() interface{} {
	buf := make([]byte, 0, 512)
	return &buf
}}

func noopPutBuffer() {}

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
			{Name: IOReadSyscall, Help: "The total number of read system calls made by the process", ValueType: prometheus.CounterValue},
			{Name: IOWriteSyscall, Help: "The total number of write system calls made by the process", ValueType: prometheus.CounterValue},
			{Name: IOReadBytes, Help: "The total number of bytes read by the process", ValueType: prometheus.CounterValue},
			{Name: IOWriteBytes, Help: "The total number of bytes written by the process", ValueType: prometheus.CounterValue},
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

	data, putBuffer, err := readFileNoStat(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return pio, err
	}
	defer putBuffer()

	return pio, parseProcIOStat(data, &pio)
}

func parseProcIOStat(data []byte, pio *procfs.ProcIO) error {
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line = data[:i]
			data = data[i+1:]
		} else {
			data = nil
		}
		if len(line) == 0 {
			continue
		}

		key, rawValue, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			return fmt.Errorf("invalid proc io line %q", line)
		}
		value, err := strconv.ParseInt(string(bytes.TrimSpace(rawValue)), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid proc io value %q: %w", rawValue, err)
		}

		switch string(key) {
		case "rchar":
			pio.RChar = uint64(value)
		case "wchar":
			pio.WChar = uint64(value)
		case "syscr":
			pio.SyscR = uint64(value)
		case "syscw":
			pio.SyscW = uint64(value)
		case "read_bytes":
			pio.ReadBytes = uint64(value)
		case "write_bytes":
			pio.WriteBytes = uint64(value)
		case "cancelled_write_bytes":
			pio.CancelledWriteBytes = value
		}
	}
	return nil
}

func readFileNoStat(filename string) ([]byte, func(), error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, noopPutBuffer, err
	}
	defer f.Close()

	bufp := procIOBufferPool.Get().(*[]byte)
	buf := (*bufp)[:0]
	reader := io.LimitReader(f, maxProcIOBufferSize)
	for {
		if len(buf) == cap(buf) {
			if cap(buf) >= maxProcIOBufferSize {
				break
			}
			newCap := cap(buf) * 2
			if newCap == 0 {
				newCap = 512
			}
			if newCap > maxProcIOBufferSize {
				newCap = maxProcIOBufferSize
			}
			next := make([]byte, len(buf), newCap)
			copy(next, buf)
			buf = next
		}
		n, err := reader.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err == io.EOF {
			break
		}
		if err != nil {
			*bufp = buf[:0]
			procIOBufferPool.Put(bufp)
			return nil, nil, err
		}
	}

	return buf, func() {
		*bufp = buf[:0]
		procIOBufferPool.Put(bufp)
	}, nil
}
