package nlqdisc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"syscall"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	log "github.com/sirupsen/logrus"
)

const (
	// nolint
	TCAUnspec = iota
	TCAKind
	TCAOptions
	TCAStats
	TCAXStats
	TCARate
	TCAFcnt
	TCAStats2
	TCAStab
	// __TCA_MAX

	TCAStatsUnspec = iota
	TCAStatsBasic
	TCAStatsRateEst
	TCAStatsQueue
	TCAStatsApp
	TCAStatsRateEst64
	// __TCAStats_MAX

	familyRoute = 0
)

var (
	probeName = "qdisc"

	Bytes      = "bytes"
	Packets    = "packets"
	Drops      = "drops"
	Qlen       = "qlen"
	Backlog    = "backlog"
	Overlimits = "overlimits"

	qdiscMetrics = []string{Bytes, Packets, Drops, Qlen, Backlog, Overlimits}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, qdiscProbeCreator)
}

func qdiscProbeCreator() (probe.MetricsProbe, error) {
	p := &Probe{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, qdiscMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type Probe struct{}

func (p *Probe) Start(_ context.Context) error {
	return nil
}

func (p *Probe) Stop(_ context.Context) error {
	return nil
}

func (p *Probe) CollectOnce() (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, metric := range qdiscMetrics {
		resMap[metric] = make(map[uint32]uint64)
	}

	ets := nettop.GetAllUniqueNetNSEntity()
	for _, et := range ets {
		stats, err := getQdiscStats(et)
		if err != nil {
			log.Errorf("%s failed get qdisc stats: %v", probeName, err)
			continue
		}

		for _, stat := range stats {
			// only care about eth0/eth1...
			if strings.HasPrefix(stat.IfaceName, "eth") {
				resMap[Bytes][uint32(et.GetNetns())] += stat.Bytes
				resMap[Packets][uint32(et.GetNetns())] += uint64(stat.Packets)
				resMap[Drops][uint32(et.GetNetns())] += uint64(stat.Drops)
				resMap[Qlen][uint32(et.GetNetns())] += uint64(stat.Qlen)
				resMap[Backlog][uint32(et.GetNetns())] += uint64(stat.Backlog)
				resMap[Overlimits][uint32(et.GetNetns())] += uint64(stat.Overlimits)
			}
		}
	}

	return resMap, nil
}

func getQdiscStats(entity *nettop.Entity) ([]QdiscInfo, error) {
	fd, err := entity.GetNetNsFd()
	if err != nil {
		return nil, err
	}

	c, err := getConn(fd)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	req := netlink.Message{
		Header: netlink.Header{
			Flags: netlink.Request | netlink.Dump,
			Type:  38, // RTM_GETQDISC
		},
		Data: make([]byte, 20),
	}

	msgs, err := c.Execute(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}

	res := []QdiscInfo{}
	for _, msg := range msgs {
		m, err := parseMessage(msg)
		if err != nil {
			log.Errorf("failed parse qdisc msg, nlmsg: %v, err: %v", msg, err)
			continue
		}
		res = append(res, m)
	}

	return res, nil
}

func getConn(nsfd int) (*netlink.Conn, error) {
	c, err := netlink.Dial(familyRoute, &netlink.Config{
		NetNS: nsfd,
	})
	if err != nil {
		return nil, err
	}

	if err := c.SetOption(netlink.GetStrictCheck, true); err != nil {
		// silently accept ENOPROTOOPT errors when kernel is not > 4.20
		if !errors.Is(err, syscall.ENOPROTOOPT) {
			return nil, fmt.Errorf("unexpected error trying to set option NETLINK_GET_STRICT_CHK: %v", err)
		}
	}

	return c, nil
}

type QdiscInfo struct {
	IfaceName   string
	Parent      uint32
	Handle      uint32
	Kind        string
	Bytes       uint64
	Packets     uint32
	Drops       uint32
	Requeues    uint32
	Overlimits  uint32
	GcFlows     uint64
	Throttled   uint64
	FlowsPlimit uint64
	Qlen        uint32
	Backlog     uint32
}

// See struct tc_stats in /usr/include/linux/pkt_sched.h
type TCStats struct {
	Bytes      uint64
	Packets    uint32
	Drops      uint32
	Overlimits uint32
	Bps        uint32
	Pps        uint32
	Qlen       uint32
	Backlog    uint32
}

// See /usr/include/linux/gen_stats.h
type TCStats2 struct {
	// struct gnet_stats_basic
	Bytes   uint64
	Packets uint32
	// struct gnet_stats_queue
	Qlen       uint32
	Backlog    uint32
	Drops      uint32
	Requeues   uint32
	Overlimits uint32
}

// See struct tc_fq_qd_stats /usr/include/linux/pkt_sched.h
type TCFqQdStats struct {
	GcFlows             uint64
	HighprioPackets     uint64
	TCPRetrans          uint64
	Throttled           uint64
	FlowsPlimit         uint64
	PktsTooLong         uint64
	AllocationErrors    uint64
	TimeNextDelayedFlow int64
	Flows               uint32
	InactiveFlows       uint32
	ThrottledFlows      uint32
	UnthrottleLatencyNs uint32
}

func parseTCAStats(attr netlink.Attribute) TCStats {
	var stats TCStats
	stats.Bytes = nlenc.Uint64(attr.Data[0:8])
	stats.Packets = nlenc.Uint32(attr.Data[8:12])
	stats.Drops = nlenc.Uint32(attr.Data[12:16])
	stats.Overlimits = nlenc.Uint32(attr.Data[16:20])
	stats.Bps = nlenc.Uint32(attr.Data[20:24])
	stats.Pps = nlenc.Uint32(attr.Data[24:28])
	stats.Qlen = nlenc.Uint32(attr.Data[28:32])
	stats.Backlog = nlenc.Uint32(attr.Data[32:36])
	return stats
}

func parseTCAStats2(attr netlink.Attribute) TCStats2 {
	var stats TCStats2

	nested, _ := netlink.UnmarshalAttributes(attr.Data)

	for _, a := range nested {
		switch a.Type {
		case TCAStatsBasic:
			stats.Bytes = nlenc.Uint64(a.Data[0:8])
			stats.Packets = nlenc.Uint32(a.Data[8:12])
		case TCAStatsQueue:
			stats.Qlen = nlenc.Uint32(a.Data[0:4])
			stats.Backlog = nlenc.Uint32(a.Data[4:8])
			stats.Drops = nlenc.Uint32(a.Data[8:12])
			stats.Requeues = nlenc.Uint32(a.Data[12:16])
			stats.Overlimits = nlenc.Uint32(a.Data[16:20])
		default:
		}
	}

	return stats
}

func parseTCFqQdStats(attr netlink.Attribute) (TCFqQdStats, error) {
	var stats TCFqQdStats

	nested, err := netlink.UnmarshalAttributes(attr.Data)
	if err != nil {
		return stats, err
	}

	pts := []*uint64{
		&stats.GcFlows,
		&stats.HighprioPackets,
		&stats.TCPRetrans,
		&stats.Throttled,
		&stats.FlowsPlimit,
		&stats.PktsTooLong,
		&stats.AllocationErrors,
	}
	for _, a := range nested {
		switch a.Type {
		case TCAStatsApp:
			for i := 0; i < len(pts) && (i+1)*8 <= len(a.Data); i++ {
				*pts[i] = nlenc.Uint64(a.Data[i*8 : (i+1)*8])
			}
		default:
		}
	}

	return stats, nil
}

// See https://tools.ietf.org/html/rfc3549#section-3.1.3
func parseMessage(msg netlink.Message) (QdiscInfo, error) {
	var m QdiscInfo
	var s TCStats
	var s2 TCStats2
	var sFq TCFqQdStats

	/*
	   struct tcmsg {
	       unsigned char   tcm_family;
	       unsigned char   tcm__pad1;
	       unsigned short  tcm__pad2;
	       int     tcm_ifindex;
	       __u32       tcm_handle;
	       __u32       tcm_parent;
	       __u32       tcm_info;
	   };
	*/

	if len(msg.Data) < 20 {
		return m, fmt.Errorf("short message, len=%d", len(msg.Data))
	}

	ifaceIdx := nlenc.Uint32(msg.Data[4:8])

	m.Handle = nlenc.Uint32(msg.Data[8:12])
	m.Parent = nlenc.Uint32(msg.Data[12:16])

	if m.Parent == math.MaxUint32 {
		m.Parent = 0
	}

	// The first 20 bytes are taken by tcmsg
	attrs, err := netlink.UnmarshalAttributes(msg.Data[20:])

	if err != nil {
		return m, fmt.Errorf("failed to unmarshal attributes: %v", err)
	}

	for _, attr := range attrs {
		switch attr.Type {
		case TCAKind:
			m.Kind = nlenc.String(attr.Data)
		case TCAStats2:
			sFq, err = parseTCFqQdStats(attr)
			if err != nil {
				return m, err
			}
			if sFq.GcFlows > 0 {
				m.GcFlows = sFq.GcFlows
			}
			if sFq.Throttled > 0 {
				m.Throttled = sFq.Throttled
			}
			if sFq.FlowsPlimit > 0 {
				m.FlowsPlimit = sFq.FlowsPlimit
			}

			s2 = parseTCAStats2(attr)
			m.Bytes = s2.Bytes
			m.Packets = s2.Packets
			m.Drops = s2.Drops
			// requeues only available in TCAStats2, not in TCAStats
			m.Requeues = s2.Requeues
			m.Overlimits = s2.Overlimits
			m.Qlen = s2.Qlen
			m.Backlog = s2.Backlog
		case TCAStats:
			// Legacy
			s = parseTCAStats(attr)
			m.Bytes = s.Bytes
			m.Packets = s.Packets
			m.Drops = s.Drops
			m.Overlimits = s.Overlimits
			m.Qlen = s.Qlen
			m.Backlog = s.Backlog
		default:
			// TODO: TCAOptions and TCAXStats
		}
	}

	iface, err := net.InterfaceByIndex(int(ifaceIdx))

	if err == nil {
		m.IfaceName = iface.Name
	}

	return m, err
}
