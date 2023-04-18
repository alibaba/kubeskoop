package nlqdisc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"syscall"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"golang.org/x/exp/slog"
)

const (
	TCA_UNSPEC = iota
	TCA_KIND
	TCA_OPTIONS
	TCA_STATS
	TCA_XSTATS
	TCA_RATE
	TCA_FCNT
	TCA_STATS2
	TCA_STAB
	// __TCA_MAX

	TCA_STATS_UNSPEC = iota
	TCA_STATS_BASIC
	TCA_STATS_RATE_EST
	TCA_STATS_QUEUE
	TCA_STATS_APP
	TCA_STATS_RATE_EST64
	// __TCA_STATS_MAX

	familyRoute = 0
)

var (
	MODULE_NAME = "insp_qdisc"

	Bytes      = "Bytes"
	Packets    = "Packets"
	Drops      = "Drops"
	Qlen       = "Qlen"
	Backlog    = "Backlog"
	Overlimits = "Overlimits"

	QdiscMetrics = []string{Bytes, Packets, Drops, Qlen, Backlog, Overlimits}

	probe = &NlQdiscProbe{mtx: sync.Mutex{}}
)

type NlQdiscProbe struct {
	enable bool
	mtx    sync.Mutex
}

func GetProbe() *NlQdiscProbe {
	return probe
}

func (p *NlQdiscProbe) Name() string {
	return MODULE_NAME
}

func (s *NlQdiscProbe) Start(_ context.Context) {
}

func (p *NlQdiscProbe) Ready() bool {
	fd, err := nettop.GetHostnetworkNetnsFd()
	if err != nil {
		slog.Default().Warn("status not ready", "err", err, "module", MODULE_NAME)
		return false
	}
	c, err := getConn(fd)
	if err != nil {
		slog.Default().Warn("status not ready", "err", err, "module", MODULE_NAME)
		return false
	}
	defer c.Close()

	return true
}

func (s *NlQdiscProbe) GetMetricNames() []string {
	res := []string{}
	for _, m := range QdiscMetrics {
		res = append(res, metricUniqueId("qdisc", m))
	}
	return res
}

func (p *NlQdiscProbe) Close() error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.enable = false

	return nil
}

func (s *NlQdiscProbe) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, metric := range QdiscMetrics {
		resMap[metricUniqueId("qdisc", metric)] = make(map[uint32]uint64)
	}

	ets := nettop.GetAllEntity()
	for _, et := range ets {
		stats, err := getQdiscStats(ctx, et)
		if err != nil {
			slog.Ctx(ctx).Info("get qdisc stats", "err", err, "module", MODULE_NAME)
			continue
		}

		for _, stat := range stats {
			// only care about eth0/eth1...
			if strings.HasPrefix(stat.IfaceName, "eth") {
				resMap[metricUniqueId("qdisc", Bytes)][uint32(et.GetNetns())] += stat.Bytes
				resMap[metricUniqueId("qdisc", Packets)][uint32(et.GetNetns())] += uint64(stat.Packets)
				resMap[metricUniqueId("qdisc", Drops)][uint32(et.GetNetns())] += uint64(stat.Drops)
				resMap[metricUniqueId("qdisc", Qlen)][uint32(et.GetNetns())] += uint64(stat.Qlen)
				resMap[metricUniqueId("qdisc", Backlog)][uint32(et.GetNetns())] += uint64(stat.Backlog)
				resMap[metricUniqueId("qdisc", Overlimits)][uint32(et.GetNetns())] += uint64(stat.Overlimits)
			}
		}
	}

	return resMap, nil
}

func getQdiscStats(ctx context.Context, entity *nettop.Entity) ([]QdiscInfo, error) {
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
			slog.Ctx(ctx).Info("parse qdisc msg", "err", err, "msg", msg, "module", MODULE_NAME)
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
type TC_Stats struct {
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
type TC_Stats2 struct {
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
type TC_Fq_Qd_Stats struct {
	GcFlows             uint64
	HighprioPackets     uint64
	TcpRetrans          uint64
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

func parseTCAStats(attr netlink.Attribute) TC_Stats {
	var stats TC_Stats
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

func parseTCAStats2(attr netlink.Attribute) TC_Stats2 {
	var stats TC_Stats2

	nested, _ := netlink.UnmarshalAttributes(attr.Data)

	for _, a := range nested {
		switch a.Type {
		case TCA_STATS_BASIC:
			stats.Bytes = nlenc.Uint64(a.Data[0:8])
			stats.Packets = nlenc.Uint32(a.Data[8:12])
		case TCA_STATS_QUEUE:
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

func parseTC_Fq_Qd_Stats(attr netlink.Attribute) (TC_Fq_Qd_Stats, error) {
	var stats TC_Fq_Qd_Stats

	nested, err := netlink.UnmarshalAttributes(attr.Data)
	if err != nil {
		return stats, err
	}

	pts := []*uint64{
		&stats.GcFlows,
		&stats.HighprioPackets,
		&stats.TcpRetrans,
		&stats.Throttled,
		&stats.FlowsPlimit,
		&stats.PktsTooLong,
		&stats.AllocationErrors,
	}
	for _, a := range nested {
		switch a.Type {
		case TCA_STATS_APP:
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
	var s TC_Stats
	var s2 TC_Stats2
	var s_fq TC_Fq_Qd_Stats

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
		case TCA_KIND:
			m.Kind = nlenc.String(attr.Data)
		case TCA_STATS2:
			s_fq, err = parseTC_Fq_Qd_Stats(attr)
			if err != nil {
				return m, err
			}
			if s_fq.GcFlows > 0 {
				m.GcFlows = s_fq.GcFlows
			}
			if s_fq.Throttled > 0 {
				m.Throttled = s_fq.Throttled
			}
			if s_fq.FlowsPlimit > 0 {
				m.FlowsPlimit = s_fq.FlowsPlimit
			}

			s2 = parseTCAStats2(attr)
			m.Bytes = s2.Bytes
			m.Packets = s2.Packets
			m.Drops = s2.Drops
			// requeues only available in TCA_STATS2, not in TCA_STATS
			m.Requeues = s2.Requeues
			m.Overlimits = s2.Overlimits
			m.Qlen = s2.Qlen
			m.Backlog = s2.Backlog
		case TCA_STATS:
			// Legacy
			s = parseTCAStats(attr)
			m.Bytes = s.Bytes
			m.Packets = s.Packets
			m.Drops = s.Drops
			m.Overlimits = s.Overlimits
			m.Qlen = s.Qlen
			m.Backlog = s.Backlog
		default:
			// TODO: TCA_OPTIONS and TCA_XSTATS
		}
	}

	iface, err := net.InterfaceByIndex(int(ifaceIdx))

	if err == nil {
		m.IfaceName = iface.Name
	}

	return m, err
}

func metricUniqueId(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}
