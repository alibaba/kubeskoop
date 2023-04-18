package nlconntrack

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/ti-mo/conntrack"
)

var (
	MetricPrefix = "conntrack"

	// stats of conntrack entry operation
	Found         = "Found"
	Invalid       = "Invalid"
	Ignore        = "Ignore"
	Insert        = "Insert"
	InsertFailed  = "InsertFailed"
	Drop          = "Drop"
	EarlyDrop     = "EarlyDrop"
	Error         = "Error"
	SearchRestart = "SearchRestart"

	Entries    = "Entries"
	MaxEntries = "MaxEntries"

	// stats of conntrack status summary

	conntrackMetrics = []string{Found, Invalid, Ignore, Insert, InsertFailed, Drop, EarlyDrop, Error, SearchRestart, Entries, MaxEntries}
)

func (s *NlConntrackProbe) GetMetricNames() []string {
	res := []string{}
	for _, m := range conntrackMetrics {
		res = append(res, metricUniqueId("conntrack", m))
	}
	return res
}

func (s *NlConntrackProbe) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	resMap := map[string]map[uint32]uint64{}
	stats, err := s.collectStats()
	if err != nil {
		return resMap, err
	}

	for _, metric := range conntrackMetrics {
		resMap[metricUniqueId("conntrack", metric)] = map[uint32]uint64{uint32(nettop.InitNetns): stats[metric]}
	}

	return resMap, nil
}

func (s *NlConntrackProbe) getConn() (*conntrack.Conn, error) {
	if s.initConn == nil {
		err := s.initStatConn()
		if err != nil {
			return nil, err
		}
	}
	return s.initConn, nil
}

func (s *NlConntrackProbe) collectStats() (map[string]uint64, error) {
	resMap := map[string]uint64{}

	conn, err := s.getConn()
	if err != nil {
		return resMap, err
	}

	stat, err := conn.Stats()
	if err != nil {
		return resMap, err
	}

	globalstat, err := conn.StatsGlobal()
	if err != nil {
		return resMap, err
	}

	for _, statpercpu := range stat {
		resMap[Found] += uint64(statpercpu.Found)
		resMap[Invalid] += uint64(statpercpu.Invalid)
		resMap[Ignore] += uint64(statpercpu.Ignore)
		resMap[Insert] += uint64(statpercpu.Insert)
		resMap[InsertFailed] += uint64(statpercpu.InsertFailed)
		resMap[Drop] += uint64(statpercpu.Drop)
		resMap[EarlyDrop] += uint64(statpercpu.EarlyDrop)
		resMap[Error] += uint64(statpercpu.Error)
		resMap[SearchRestart] += uint64(statpercpu.SearchRestart)
	}

	resMap[Entries] = uint64(globalstat.Entries)
	resMap[MaxEntries] = uint64(globalstat.MaxEntries)

	return resMap, nil
}

// initStatConn create a netlink connection in init netns
func (s *NlConntrackProbe) initStatConn() error {
	c, err := conntrack.Dial(nil)
	if err != nil {
		return err
	}
	s.initConn = c
	return nil
}

func metricUniqueId(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}
