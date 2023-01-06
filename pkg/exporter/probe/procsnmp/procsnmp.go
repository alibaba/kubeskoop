package procsnmp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	nettop2 "github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"golang.org/x/exp/slog"
)

const (
	ProtocolICMP     = "Icmp"
	ProtocolICMPMsg  = "IcmpMsg"
	ProtocolIP       = "Ip"
	ProtocolIPExt    = "IpExt"
	ProtocolMPTCPExt = "MPTcpExt"
	ProtocolTCP      = "Tcp"
	ProtocolTCPExt   = "TcpExt"
	ProtocolUDP      = "Udp"
	ProtocolUDPLite  = "UdpLite"

	// metrics of tcp
	TCPActiveOpens     = "ActiveOpens"
	TCPPassiveOpens    = "PassiveOpens"
	TCPRetransSegs     = "RetransSegs"
	TCPListenDrops     = "ListenDrops"
	TCPListenOverflows = "ListenOverflows"
	TCPSynRetrans      = "TCPSynRetrans"
	TCPFastRetrans     = "TCPFastRetrans"
	TCPRetransFail     = "TCPRetransFail"
	TCPTimeouts        = "TCPTimeouts"
	TCPAttemptFails    = "AttemptFails"
	TCPEstabResets     = "EstabResets"
	TCPCurrEstab       = "CurrEstab"
	TCPInSegs          = "InSegs"
	TCPOutSegs         = "OutSegs"
	TCPInErrs          = "InErrs"
	TCPOutRsts         = "OutRsts"

	// metrics of udp
	UDPInDatagrams  = "InDatagrams"
	UDPNoPorts      = "NoPorts"
	UDPInErrors     = "InErrors"
	UDPOutDatagrams = "OutDatagrams"
	UDPRcvbufErrors = "RcvbufErrors"
	UDPSndbufErrors = "SndbufErrors"
	UDPInCsumErrors = "InCsumErrors"
	UDPIgnoredMulti = "IgnoredMulti"

	//metrics of ip
	IPInNoRoutes      = "InNoRoutes"
	IPInTruncatedPkts = "InTruncatedPkts"

	MODULE_NAME = "procsnmp" // nolint
)

var (
	TCPStatMetrcis = []string{TCPActiveOpens, TCPPassiveOpens, TCPRetransSegs, TCPAttemptFails, TCPEstabResets, TCPCurrEstab, TCPInSegs, TCPOutSegs, TCPInErrs, TCPOutRsts}
	UDPStatMetrics = []string{UDPInDatagrams, UDPNoPorts, UDPInErrors, UDPOutDatagrams, UDPRcvbufErrors, UDPSndbufErrors, UDPInCsumErrors, UDPIgnoredMulti}
	IPMetrics      = []string{IPInNoRoutes, IPInTruncatedPkts}

	probe *ProcSNMP
	once  sync.Once
)

func GetProbe() *ProcSNMP {
	once.Do(func() {
		probe = &ProcSNMP{}
	})
	return probe
}

type ProcSNMP struct {
}

func (s *ProcSNMP) Close() error {
	return nil
}

func (s *ProcSNMP) Start(_ context.Context) {
}

func (s *ProcSNMP) Ready() bool {
	// determine by if default snmp file was ready
	if _, err := os.Stat("/proc/net/snmp"); os.IsNotExist(err) {
		return false
	}
	return true
}

func (s *ProcSNMP) Name() string {
	return MODULE_NAME
}

func (s *ProcSNMP) GetMetricNames() []string {
	res := []string{}
	for _, m := range TCPStatMetrcis {
		res = append(res, fmt.Sprintf("tcp%s", strings.ToLower(m)))
	}
	for _, m := range UDPStatMetrics {
		res = append(res, fmt.Sprintf("udp%s", strings.ToLower(m)))
	}
	for _, m := range IPMetrics {
		res = append(res, fmt.Sprintf("ip%s", strings.ToLower(m)))
	}
	return res
}

func (s *ProcSNMP) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop2.GetAllEntity()
	if len(ets) == 0 {
		slog.Ctx(ctx).Info("collect", "mod", MODULE_NAME, "ignore", "no entity found")
	}
	return collect(ctx, ets)
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", strings.ToLower(subject), strings.ToLower(m))
}

func collect(ctx context.Context, entitys []*nettop2.Entity) (map[string]map[uint32]uint64, error) {
	res := map[string]map[uint32]uint64{}
	for _, m := range TCPStatMetrcis {
		res[metricUniqueID("tcp", m)] = map[uint32]uint64{}
	}
	for _, m := range UDPStatMetrics {
		res[metricUniqueID("udp", m)] = map[uint32]uint64{}
	}
	for _, m := range IPMetrics {
		res[metricUniqueID("ip", m)] = map[uint32]uint64{}
	}

	for _, et := range entitys {
		if et != nil {
			pid := et.GetPid()
			nsinum := et.GetNetns()

			stats, err := getNetstatByPid(pid)
			if err != nil {
				slog.Ctx(ctx).Debug("get netstat failed", "pid", pid, "nsinum", nsinum, "err", err)
				continue
			}

			for proto, stat := range stats {
				for k, v := range stat {
					mkey := metricUniqueID(proto, k)
					slog.Ctx(ctx).Debug("store metric", "metric", mkey, "pid", pid, "nsinum", nsinum, "value", v)
					if data, err := strconv.ParseInt(v, 10, 64); err != nil {
						slog.Ctx(ctx).Debug("parse netstat value", "metric", mkey, "pid", pid, "nsinum", nsinum, "value", v, "err", err)
						continue
					} else {
						// ignore unaware metric
						if _, ok := res[mkey]; ok {
							res[mkey][uint32(nsinum)] = uint64(data)
						}
					}
				}
			}
		}
	}

	return res, nil
}

func getNetstatByPid(pid int) (map[string]map[string]string, error) {
	resMap := make(map[string]map[string]string)

	snmppath := fmt.Sprintf("/proc/%d/net/snmp", pid)
	if _, err := os.Stat(snmppath); os.IsNotExist(err) {
		return resMap, err
	}
	snmpStats, err := getNetStats(snmppath)
	if err != nil {
		return resMap, err
	}

	for k, v := range snmpStats {
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
