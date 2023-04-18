package procipvs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const maxBufferSize = 1024 * 1024

var (
	ModuleName = "insp_ipvs"

	statf = "/proc/net/ip_vs_stats"

	probe = &ProcIPVS{}

	IPVSServiceCount              = "IPVSServiceCount"
	IPVSServiceTCPConnCount       = "IPVSServiceTCPConnCount"
	IPVSServiceTCPInBytesCount    = "IPVSServiceTCPInBytesCount"
	IPVSServiceTCPInPacketsCount  = "IPVSServiceTCPInPacketsCount"
	IPVSServiceTCPOutBytesCount   = "IPVSServiceTCPOutBytesCount"
	IPVSServiceTCPOutPacketsCount = "IPVSServiceTCPOutPacketsCount"
	IPVSServiceUDPConnCount       = "IPVSServiceUDPConnCount"
	IPVSServiceUDPInBytesCount    = "IPVSServiceUDPInBytesCount"
	IPVSServiceUDPInPacketsCount  = "IPVSServiceUDPInPacketsCount"
	IPVSServiceUDPOutBytesCount   = "IPVSServiceUDPOutBytesCount"
	IPVSServiceUDPOutPacketsCount = "IPVSServiceUDPOutPacketsCount"

	Connections     = "Connections"
	IncomingPackets = "IncomingPackets"
	OutgoingPackets = "OutgoingPackets"
	IncomingBytes   = "IncomingBytes"
	OutgoingBytes   = "OutgoingBytes"

	IPVSMetrics = []string{Connections, IncomingPackets, OutgoingBytes, IncomingBytes, OutgoingPackets}
)

type ProcIPVS struct {
}

func GetProbe() *ProcIPVS {
	return probe
}

func (p *ProcIPVS) Name() string {
	return ModuleName
}

func (p *ProcIPVS) Close() error {
	return nil
}

func (p *ProcIPVS) Start(_ context.Context) {
}

func (p *ProcIPVS) Ready() bool {
	// determine by if default ipvs stats file was ready
	if _, err := os.Stat(statf); os.IsNotExist(err) {
		return false
	}
	return true
}

func (p *ProcIPVS) GetMetricNames() []string {
	res := []string{}
	for _, m := range IPVSMetrics {
		res = append(res, metricUniqueID("ipvs", m))
	}
	return res
}

func (p *ProcIPVS) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	f, err := os.Open(statf)
	if err != nil {
		return resMap, err
	}
	defer f.Close()

	reader := io.LimitReader(f, maxBufferSize)
	data, err := io.ReadAll(reader)
	if err != nil {
		return resMap, err
	}

	stats, err := parseIPVSStats(bytes.NewReader(data))
	if err != nil {
		return resMap, err
	}
	// only handle stats in default netns
	resMap[metricUniqueID("ipvs", Connections)] = map[uint32]uint64{0: stats.Connections}
	resMap[metricUniqueID("ipvs", IncomingPackets)] = map[uint32]uint64{0: stats.IncomingBytes}
	resMap[metricUniqueID("ipvs", IncomingBytes)] = map[uint32]uint64{0: stats.IncomingBytes}
	resMap[metricUniqueID("ipvs", OutgoingPackets)] = map[uint32]uint64{0: stats.OutgoingPackets}
	resMap[metricUniqueID("ipvs", OutgoingBytes)] = map[uint32]uint64{0: stats.OutgoingBytes}
	return resMap, nil
}

func metricUniqueID(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

// IPVSStats holds IPVS statistics, as exposed by the kernel in `/proc/net/ip_vs_stats`.
type IPVSStats struct {
	// Total count of connections.
	Connections uint64
	// Total incoming packages processed.
	IncomingPackets uint64
	// Total outgoing packages processed.
	OutgoingPackets uint64
	// Total incoming traffic.
	IncomingBytes uint64
	// Total outgoing traffic.
	OutgoingBytes uint64
}

// parseIPVSStats performs the actual parsing of `ip_vs_stats`.
func parseIPVSStats(r io.Reader) (IPVSStats, error) {
	var (
		statContent []byte
		statLines   []string
		statFields  []string
		stats       IPVSStats
	)

	statContent, err := io.ReadAll(r)
	if err != nil {
		return IPVSStats{}, err
	}

	statLines = strings.SplitN(string(statContent), "\n", 4)
	if len(statLines) != 4 {
		return IPVSStats{}, errors.New("ip_vs_stats corrupt: too short")
	}

	statFields = strings.Fields(statLines[2])
	if len(statFields) != 5 {
		return IPVSStats{}, errors.New("ip_vs_stats corrupt: unexpected number of fields")
	}

	stats.Connections, err = strconv.ParseUint(statFields[0], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.IncomingPackets, err = strconv.ParseUint(statFields[1], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.OutgoingPackets, err = strconv.ParseUint(statFields[2], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.IncomingBytes, err = strconv.ParseUint(statFields[3], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.OutgoingBytes, err = strconv.ParseUint(statFields[4], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}

	return stats, nil
}
