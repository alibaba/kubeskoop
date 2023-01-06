package procio

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	nettop2 "github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/prometheus/procfs"
	"golang.org/x/exp/slog"
)

const (
	IOReadSyscall  = "IOReadSyscall"
	IOWriteSyscall = "IOWriteSyscall"
	IOReadBytes    = "IOReadBytes"
	IOWriteBytes   = "IOWriteBytes"

	MODULE_NAME = "procio" // nolint
)

var (
	probe = &ProcIO{}

	IOMetrics = []string{IOReadSyscall, IOWriteSyscall, IOReadBytes, IOWriteBytes}
)

type ProcIO struct {
}

func GetProbe() *ProcIO {
	return probe
}

func (s *ProcIO) Close() error {
	return nil
}

func (s *ProcIO) Start(_ context.Context) {
}

func (s *ProcIO) Ready() bool {
	return true
}

func (s *ProcIO) Name() string {
	return MODULE_NAME
}

func (s *ProcIO) GetMetricNames() []string {
	res := []string{}
	for _, m := range IOMetrics {
		res = append(res, metricUniqueID("io", m))
	}
	return res
}

func (s *ProcIO) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop2.GetAllEntity()
	if len(ets) == 0 {
		slog.Ctx(ctx).Info("collect", "mod", MODULE_NAME, "ignore", "no entity found")
	}
	return collect(ctx, ets)
}

func collect(ctx context.Context, nslist []*nettop2.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, stat := range IOMetrics {
		resMap[metricUniqueID("io", stat)] = map[uint32]uint64{}
	}

	procios, err := getAllProcessIO(nettop2.GetAllEntity())
	if err != nil {
		return resMap, err
	}

	for nsinum := range procios {
		for _, procio := range procios[nsinum] {
			resMap[metricUniqueID("io", IOReadSyscall)][nsinum] += procio.SyscR
			resMap[metricUniqueID("io", IOWriteSyscall)][nsinum] += procio.SyscW
			resMap[metricUniqueID("io", IOReadBytes)][nsinum] += procio.ReadBytes
			resMap[metricUniqueID("io", IOWriteBytes)][nsinum] += procio.WriteBytes
		}
	}

	return resMap, nil
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

func getAllProcessIO(nslist []*nettop2.Entity) (map[uint32][]procfs.ProcIO, error) {
	allprocio := make(map[uint32][]procfs.ProcIO)
	for idx := range nslist {
		nslogic := nslist[idx]
		prociolist := []procfs.ProcIO{}
		for _, indx := range nslogic.GetPids() {
			iodata, err := getProccessIoStat(indx)
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
func getProccessIoStat(pid int) (procfs.ProcIO, error) {
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
