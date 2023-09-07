package procnetstat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	log "github.com/sirupsen/logrus"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
)

const (
	probeName = "tcpext" // nolint

	ProtocolTCPExt = "tcpext"

	TCPActiveOpens     = "activeopens"
	TCPPassiveOpens    = "passiveopens"
	TCPRetransSegs     = "retranssegs"
	TCPListenDrops     = "listendrops"
	TCPListenOverflows = "listenoverflows"
	TCPSynRetrans      = "tcpsynretrans"
	TCPFastRetrans     = "tcpfastretrans"
	TCPRetransFail     = "tcpretransfail"
	TCPTimeouts        = "tcptimeouts"

	TCPAbortOnClose        = "tcpabortonclose"
	TCPAbortOnMemory       = "tcpabortonmemory"
	TCPAbortOnTimeout      = "tcpabortontimeout"
	TCPAbortOnLinger       = "tcpabortonlinger"
	TCPAbortOnData         = "tcpabortondata"
	TCPAbortFailed         = "tcpabortfailed"
	TCPACKSkippedSynRecv   = "tcpackskippedsynrecv"
	TCPACKSkippedPAWS      = "tcpackskippedpaws"
	TCPACKSkippedSeq       = "tcpackskippedseq"
	TCPACKSkippedFinWait2  = "tcpackskippedfinwait2"
	TCPACKSkippedTimeWait  = "tcpackskippedtimewait"
	TCPACKSkippedChallenge = "tcpackskippedchallenge"
	TCPRcvQDrop            = "tcprcvqdrop"
	PAWSActive             = "pawsactive"
	PAWSEstab              = "pawsestab"
	EmbryonicRsts          = "embryonicrsts"
	TCPWinProbe            = "tcpwinprobe"
	TCPKeepAlive           = "tcpkeepalive"
	TCPMTUPFail            = "tcpmtupfail"
	TCPMTUPSuccess         = "tcpmtupsuccess"
	TCPZeroWindowDrop      = "tcpzerowindowdrop"
	TCPBacklogDrop         = "tcpbacklogdrop"
	PFMemallocDrop         = "pfmemallocdrop"
	TCPWqueueTooBig        = "tcpwqueuetoobig"

	TCPMemoryPressures       = "tcpmemorypressures"
	TCPMemoryPressuresChrono = "tcpmemorypressureschrono"
)

var (
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

func init() {
	probe.MustRegisterMetricsProbe(probeName, netdevProbeCreator)
}

func netdevProbeCreator(_ map[string]interface{}) (probe.MetricsProbe, error) {
	p := &ProcNetstat{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, TCPExtMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcNetstat struct {
}

func (s *ProcNetstat) Start(_ context.Context) error {
	return nil
}

func (s *ProcNetstat) Stop(_ context.Context) error {
	return nil
}

func (s *ProcNetstat) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		log.Errorf("%s error, no entity found", probeName)
	}
	return collect(ets)
}

func collect(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)

	for _, stat := range TCPExtMetrics {
		resMap[stat] = make(map[uint32]uint64)
	}

	for _, et := range nslist {
		stats, err := getNetstatByPid(uint32(et.GetPid()))
		if err != nil {
			log.Errorf("%s failed collect pid %d, err: %v", probeName, et.GetPid(), err)
			continue
		}

		extstats := stats[ProtocolTCPExt]
		for _, stat := range TCPExtMetrics {
			if _, ok := extstats[stat]; ok {
				data, err := strconv.ParseUint(extstats[stat], 10, 64)
				if err != nil {
					log.Errorf("%s failed parse stat %s, pid: %d err: %v", probeName, stat, et.GetPid(), err)
					continue
				}
				resMap[stat][uint32(et.GetNetns())] += data
			}
		}
	}

	return resMap, nil
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
		protocol := strings.ToLower(nameParts[0][:len(nameParts[0])-1])
		netStats[protocol] = map[string]string{}
		if len(nameParts) != len(valueParts) {
			return nil, fmt.Errorf("mismatch field count mismatch in %s: %s",
				fileName, protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			netStats[protocol][strings.ToLower(nameParts[i])] = valueParts[i]
		}
	}

	return netStats, scanner.Err()
}
