package procnetstat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"golang.org/x/exp/slog"
)

const (
	ModuleName = "procnetstat" // nolint

	ProtocolTCPExt = "TcpExt"

	TCPActiveOpens     = "ActiveOpens"
	TCPPassiveOpens    = "PassiveOpens"
	TCPRetransSegs     = "RetransSegs"
	TCPListenDrops     = "ListenDrops"
	TCPListenOverflows = "ListenOverflows"
	TCPSynRetrans      = "TCPSynRetrans"
	TCPFastRetrans     = "TCPFastRetrans"
	TCPRetransFail     = "TCPRetransFail"
	TCPTimeouts        = "TCPTimeouts"

	TCPAbortOnClose        = "TCPAbortOnClose"
	TCPAbortOnMemory       = "TCPAbortOnMemory"
	TCPAbortOnTimeout      = "TCPAbortOnTimeout"
	TCPAbortOnLinger       = "TCPAbortOnLinger"
	TCPAbortOnData         = "TCPAbortOnData"
	TCPAbortFailed         = "TCPAbortFailed"
	TCPACKSkippedSynRecv   = "TCPACKSkippedSynRecv"
	TCPACKSkippedPAWS      = "TCPACKSkippedPAWS"
	TCPACKSkippedSeq       = "TCPACKSkippedSeq"
	TCPACKSkippedFinWait2  = "TCPACKSkippedFinWait2"
	TCPACKSkippedTimeWait  = "TCPACKSkippedTimeWait"
	TCPACKSkippedChallenge = "TCPACKSkippedChallenge"
	TCPRcvQDrop            = "TCPRcvQDrop"
	PAWSActive             = "PAWSActive"
	PAWSEstab              = "PAWSEstab"
	EmbryonicRsts          = "EmbryonicRsts"
	TCPWinProbe            = "TCPWinProbe"
	TCPKeepAlive           = "TCPKeepAlive"
	TCPMTUPFail            = "TCPMTUPFail"
	TCPMTUPSuccess         = "TCPMTUPSuccess"
	TCPZeroWindowDrop      = "TCPZeroWindowDrop"
	TCPBacklogDrop         = "TCPBacklogDrop"
	PFMemallocDrop         = "PFMemallocDrop"
	TCPWqueueTooBig        = "TCPWqueueTooBig"

	TCPMemoryPressures       = "TCPMemoryPressures"
	TCPMemoryPressuresChrono = "TCPMemoryPressuresChrono"
)

var (
	probe = &ProcNetstat{}

	TCPExtMetrics = []string{TCPListenDrops,
		TCPListenOverflows,
		TCPSynRetrans,
		TCPFastRetrans,
		TCPRetransFail,
		TCPTimeouts,
		TCPAbortOnClose,
		TCPAbortOnMemory,
		TCPAbortOnTimeout,
		TCPAbortOnLinger,
		TCPAbortOnData,
		TCPAbortFailed,
		TCPACKSkippedSynRecv,
		TCPACKSkippedPAWS,
		TCPACKSkippedSeq,
		TCPACKSkippedFinWait2,
		TCPACKSkippedTimeWait,
		TCPACKSkippedChallenge,
		TCPRcvQDrop,
		TCPMemoryPressures,
		TCPMemoryPressuresChrono,
		PAWSActive,
		PAWSEstab,
		EmbryonicRsts,
		TCPWinProbe,
		TCPKeepAlive,
		TCPMTUPFail,
		TCPMTUPSuccess,
		TCPZeroWindowDrop,
		TCPBacklogDrop,
		PFMemallocDrop,
		TCPWqueueTooBig}
)

type ProcNetstat struct {
}

func GetProbe() *ProcNetstat {
	return probe
}

func (s *ProcNetstat) Close() error {
	return nil
}

func (s *ProcNetstat) Start(_ context.Context) {
}

func (s *ProcNetstat) Ready() bool {
	// determine by if default snmp file was ready
	if _, err := os.Stat("/proc/net/netstat"); os.IsNotExist(err) {
		return false
	}
	return true
}

func (s *ProcNetstat) Name() string {
	return ModuleName
}

func (s *ProcNetstat) GetMetricNames() []string {
	res := []string{}
	for _, m := range TCPExtMetrics {
		res = append(res, metricUniqueID("tcpext", m))
	}
	return res
}

func (s *ProcNetstat) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		slog.Ctx(ctx).Info("collect", "mod", ModuleName, "ignore", "no entity found")
		return nil, errors.New("no entity to collect")
	}
	return collect(ctx, ets)
}

func collect(ctx context.Context, nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, stat := range TCPExtMetrics {
		resMap[metricUniqueID("tcpext", stat)] = map[uint32]uint64{}
	}

	for _, et := range nslist {
		stats, err := getNetstatByPid(uint32(et.GetPid()))
		if err != nil {
			slog.Ctx(ctx).Info("collect", "mod", ModuleName, "ignore", "no entity found")
			continue
		}
		slog.Ctx(ctx).Debug("collect", "mod", ModuleName, "netns", et.GetNetns(), "stats", stats)
		extstats := stats[ProtocolTCPExt]
		for _, stat := range TCPExtMetrics {
			if _, ok := extstats[stat]; ok {
				data, err := strconv.ParseUint(extstats[stat], 10, 64)
				if err != nil {
					slog.Ctx(ctx).Warn("collect", "mod", ModuleName, "ignore", stat, "err", err)
					continue
				}
				resMap[metricUniqueID("tcpext", stat)][uint32(et.GetNetns())] += data
			}
		}
	}

	return resMap, nil
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

func getNetstatByPid(pid uint32) (map[string]map[string]string, error) {
	resMap := make(map[string]map[string]string)
	netstatpath := fmt.Sprintf("/proc/%d/net/netstat", pid)
	if _, err := os.Stat(netstatpath); os.IsNotExist(err) {
		return resMap, err
	}

	netStats, err := getNetStats(netstatpath)
	if err != nil {
		return resMap, err
	}

	for k, v := range netStats {
		resMap[k] = v
	}

	return resMap, nil
}

func getNetStats(fileName string) (map[string]map[string]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseNetStats(file, fileName)
}

func parseNetStats(r io.Reader, fileName string) (map[string]map[string]string, error) {
	var (
		netStats = map[string]map[string]string{}
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		nameParts := strings.Split(scanner.Text(), " ")
		scanner.Scan()
		valueParts := strings.Split(scanner.Text(), " ")
		// Remove trailing :.
		protocol := nameParts[0][:len(nameParts[0])-1]
		netStats[protocol] = map[string]string{}
		if len(nameParts) != len(valueParts) {
			return nil, fmt.Errorf("mismatch field count mismatch in %s: %s",
				fileName, protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			netStats[protocol][nameParts[i]] = valueParts[i]
		}
	}

	return netStats, scanner.Err()
}
