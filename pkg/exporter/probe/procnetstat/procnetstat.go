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
	TCPExtMetrics = []probe.LegacyMetric{
		{Name: TCPListenDrops, Help: "The total number of TCP connection requests that were dropped because the listen queue was full."},
		{Name: TCPListenOverflows, Help: "The total number of times the TCP listen queue has overflown."},
		{Name: TCPSynRetrans, Help: "The total number of SYN packets that were retransmitted."},
		{Name: TCPFastRetrans, Help: "The total number of fast retransmissions made by TCP."},
		{Name: TCPRetransFail, Help: "The total number of failed retransmissions in TCP."},
		{Name: TCPTimeouts, Help: "The total number of TCP timeouts."},
		{Name: TCPAbortOnClose, Help: "The number of TCP connections that were aborted on close."},
		{Name: TCPAbortOnMemory, Help: "The number of TCP connections that were aborted due to memory allocation failures."},
		{Name: TCPAbortOnTimeout, Help: "The number of TCP connections that were aborted due to timeouts."},
		{Name: TCPAbortOnLinger, Help: "The number of TCP connections that were aborted due to linger timeouts."},
		{Name: TCPAbortOnData, Help: "The number of TCP connections that were aborted due to data-related issues."},
		{Name: TCPAbortFailed, Help: "The number of attempts to abort TCP connections that failed."},
		{Name: TCPACKSkippedSynRecv, Help: "The number of ACKs skipped while in SYN_RECV state."},
		{Name: TCPACKSkippedPAWS, Help: "The number of ACKs skipped due to PAWS (Protection Against Wrapped Sequence numbers)."},
		{Name: TCPACKSkippedSeq, Help: "The number of ACKs skipped due to sequence number issues."},
		{Name: TCPACKSkippedFinWait2, Help: "The number of ACKs skipped while in FIN_WAIT_2 state."},
		{Name: TCPACKSkippedTimeWait, Help: "The number of ACKs skipped while in TIME_WAIT state."},
		{Name: TCPACKSkippedChallenge, Help: "The number of ACKs skipped due to challenges in the communication."},
		{Name: TCPRcvQDrop, Help: "The total number of received packets that were dropped due to queue overflow."},
		{Name: TCPMemoryPressures, Help: "The total number of occasions where the TCP stack experienced memory pressure."},
		{Name: TCPMemoryPressuresChrono, Help: "Chronological count of TCP memory pressure events."},
		{Name: PAWSActive, Help: "Indicates whether the PAWS mechanism is active."},
		{Name: PAWSEstab, Help: "The number of established connections utilizing PAWS."},
		{Name: EmbryonicRsts, Help: "The number of embryonic (half-open) connections that were reset."},
		{Name: TCPWinProbe, Help: "The total number of window probes sent to check for window size."},
		{Name: TCPKeepAlive, Help: "The total number of TCP keepalive packets sent."},
		{Name: TCPMTUPFail, Help: "The total number of MTU (Maximum Transmission Unit) probe failures."},
		{Name: TCPMTUPSuccess, Help: "The total number of successful MTU (Maximum Transmission Unit) discoveries."},
		{Name: TCPZeroWindowDrop, Help: "The total number of packets dropped due to a zero window condition."},
		{Name: TCPBacklogDrop, Help: "The total number of packets dropped from the TCP backlog queue."},
		{Name: PFMemallocDrop, Help: "The total number of packets dropped due to PF_MEMALLOC allocations failing."},
		{Name: TCPWqueueTooBig, Help: "The total number of TCP send queue drops due to the queue being too large."},
	}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, netdevProbeCreator)
}

func netdevProbeCreator() (probe.MetricsProbe, error) {
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
	ets := nettop.GetAllUniqueNetnsEntity()
	if len(ets) == 0 {
		log.Errorf("%s error, no entity found", probeName)
	}
	return collect(ets)
}

func collect(nslist []*nettop.Entity) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)

	for _, stat := range TCPExtMetrics {
		resMap[stat.Name] = make(map[uint32]uint64)
	}

	for _, et := range nslist {
		stats, err := getNetstatByPid(uint32(et.GetPid()))
		if err != nil {
			log.Errorf("%s failed collect pid %d, err: %v", probeName, et.GetPid(), err)
			continue
		}

		extstats := stats[ProtocolTCPExt]
		for _, stat := range TCPExtMetrics {
			if _, ok := extstats[stat.Name]; ok {
				data, err := strconv.ParseUint(extstats[stat.Name], 10, 64)
				if err != nil {
					log.Errorf("%s failed parse stat %s, pid: %d err: %v", probeName, stat, et.GetPid(), err)
					continue
				}
				resMap[stat.Name][uint32(et.GetNetns())] += data
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
