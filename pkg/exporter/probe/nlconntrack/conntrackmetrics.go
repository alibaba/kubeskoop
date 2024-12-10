package nlconntrack

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/ti-mo/conntrack"
)

var (
	MetricPrefix = "conntrack"

	// stats of conntrack entry operation
	Found         = "found"
	Invalid       = "invalid"
	Ignore        = "ignore"
	Insert        = "insert"
	InsertFailed  = "insertfailed"
	Drop          = "drop"
	EarlyDrop     = "earlydrop"
	Error         = "error"
	SearchRestart = "searchrestart"

	Entries    = "entries"
	MaxEntries = "maxentries"

	// stats of conntrack status summary
	conntrackMetrics = []probe.LegacyMetric{
		{Name: Found, Help: "The total number of tracked connections found in the conntrack table."},
		{Name: Invalid, Help: "The total number of invalid connections encountered."},
		{Name: Ignore, Help: "The total number of connections that were ignored by the conntrack module."},
		{Name: Insert, Help: "The total number of connections inserted into the conntrack table."},
		{Name: InsertFailed, Help: "The total number of failed attempts to insert a connection into the conntrack table."},
		{Name: Drop, Help: "The total number of connections dropped from the conntrack table."},
		{Name: EarlyDrop, Help: "The total number of connections dropped early before they were fully established."},
		{Name: Error, Help: "The total number of errors encountered while managing connections in the conntrack table."},
		{Name: SearchRestart, Help: "The total number of times the search for a connection entry was restarted."},
		{Name: Entries, Help: "The current number of connections tracked in the conntrack table."},
		{Name: MaxEntries, Help: "The maximum number of entries allowed in the conntrack table."},
	}
)

func metricsProbeCreator() (probe.MetricsProbe, error) {
	p := &conntrackMetricsProbe{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, conntrackMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type conntrackMetricsProbe struct {
	conn *conntrack.Conn
}

func (c *conntrackMetricsProbe) collectStats() (map[string]uint64, error) {
	resMap := map[string]uint64{}

	stat, err := c.conn.Stats()
	if err != nil {
		return resMap, err
	}

	globalstat, err := c.conn.StatsGlobal()
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

func (c *conntrackMetricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	resMap := map[string]map[uint32]uint64{}
	stats, err := c.collectStats()
	if err != nil {
		return resMap, err
	}

	for _, metric := range conntrackMetrics {
		resMap[metric.Name] = map[uint32]uint64{uint32(nettop.InitNetns): stats[metric.Name]}
	}

	return resMap, nil
}

func (c *conntrackMetricsProbe) Start(_ context.Context) error {
	var err error
	c.conn, err = conntrack.Dial(nil)
	return err
}

func (c *conntrackMetricsProbe) Stop(_ context.Context) error {
	return c.conn.Close()
}

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
}
