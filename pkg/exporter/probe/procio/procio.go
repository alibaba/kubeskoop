package procio

import (
	"context"
	"fmt"
	"io"
	"os"

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

var (
	IOMetrics = []string{IOReadSyscall, IOWriteSyscall, IOReadBytes, IOWriteBytes}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, ioProbeCreator)
}

func ioProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcIO{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, IOMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcIO struct {
}

func (s *ProcIO) Start(_ context.Context) error {
	return nil
}

func (s *ProcIO) Stop(_ context.Context) error {
	return nil
}

func (s *ProcIO) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		log.Infof("procio: no entity found")
	}
	return collect(ets)
}

func collect(_ []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, stat := range IOMetrics {
		resMap[stat] = map[uint32]uint64{}
	}

	procios, err := getAllProcessIO(nettop.GetAllEntity())
	if err != nil {
		return resMap, err
	}

	for nsinum := range procios {
		for _, procio := range procios[nsinum] {
			resMap[IOReadSyscall][nsinum] += procio.SyscR
			resMap[IOWriteSyscall][nsinum] += procio.SyscW
			resMap[IOReadBytes][nsinum] += procio.ReadBytes
			resMap[IOWriteBytes][nsinum] += procio.WriteBytes
		}
	}

	return resMap, nil
}

func getAllProcessIO(nslist []*nettop.Entity) (map[uint32][]procfs.ProcIO, error) {
	allprocio := make(map[uint32][]procfs.ProcIO)
	for idx := range nslist {
		nslogic := nslist[idx]
		prociolist := []procfs.ProcIO{}
		for _, indx := range nslogic.GetPids() {
			iodata, err := getProcessIOStat(indx)
			if err != nil {
				continue
			}
			prociolist = append(prociolist, iodata)
		}
		allprocio[uint32(nslogic.GetNetns())] = prociolist
	}
	return allprocio, nil
}

// IO creates a new ProcIO instance from a given Proc instance.
func getProcessIOStat(pid int) (procfs.ProcIO, error) {
	pio := procfs.ProcIO{}

	data, err := ReadFileNoStat(fmt.Sprintf("/proc/%d/io", pid))
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

func ReadFileNoStat(filename string) ([]byte, error) {
	const maxBufferSize = 1024 * 512

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := io.LimitReader(f, maxBufferSize)
	return io.ReadAll(reader)
}
