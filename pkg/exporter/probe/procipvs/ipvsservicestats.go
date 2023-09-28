package procipvs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

const maxBufferSize = 1024 * 1024

var (
	probeName = "ipvs"

	statf = "/proc/net/ip_vs_stats"

	Connections     = "connections"
	IncomingPackets = "incomingpackets"
	OutgoingPackets = "outgoingpackets"
	IncomingBytes   = "incomingbytes"
	OutgoingBytes   = "outgoingbytes"

	IPVSMetrics = []string{Connections, IncomingPackets, OutgoingBytes, IncomingBytes, OutgoingPackets}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, ipvsProbeCreator)
}

func ipvsProbeCreator(_ map[string]interface{}) (probe.MetricsProbe, error) {
	p := &ProcIPVS{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, IPVSMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcIPVS struct {
}

func (p *ProcIPVS) Start(_ context.Context) error {
	return nil
}

func (p *ProcIPVS) Stop(_ context.Context) error {
	return nil
}

func (p *ProcIPVS) CollectOnce() (map[string]map[uint32]uint64, error) {
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
	resMap[Connections] = map[uint32]uint64{0: stats.Connections}
	resMap[IncomingPackets] = map[uint32]uint64{0: stats.IncomingBytes}
	resMap[IncomingBytes] = map[uint32]uint64{0: stats.IncomingBytes}
	resMap[OutgoingPackets] = map[uint32]uint64{0: stats.OutgoingPackets}
	resMap[OutgoingBytes] = map[uint32]uint64{0: stats.OutgoingBytes}
	return resMap, nil
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
