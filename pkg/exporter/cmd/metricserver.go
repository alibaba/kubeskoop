package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	nettop2 "github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
	"github.com/patrickmn/go-cache"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slog"
)

var (
	CollectLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "inspector_runtime_collectlatency",
		Help: "net-exporter metrics collect latency",
	},
		[]string{"node", "probe"},
	)

	cacheUpdateInterval = 10 * time.Second
)

func NewMServer(ctx context.Context, config MetricConfig) *MServer {
	ms := &MServer{
		ctx:         ctx,
		descs:       make(map[string]*prometheus.Desc),
		config:      config,
		probes:      make(map[string]proto.MetricProbe),
		metricCache: cache.New(3*cacheUpdateInterval, 10*cacheUpdateInterval),
	}

	if len(config.Probes) == 0 {
		// if no probes configured, keep loop channel empty
		slog.Ctx(ctx).Info("new mserver with no probe required")
		return ms
	}

	for _, p := range config.Probes {
		mp := probe.GetProbe(p)
		if mp == nil {
			slog.Ctx(ctx).Info("get metric probe nil", "probe", p)
			continue
		}
		ms.probes[p] = mp
		slog.Ctx(ctx).Debug("new mserver add subject", "subject", p)
	}

	for sub, mp := range ms.probes {
		mnames := mp.GetMetricNames()
		for _, mname := range mnames {
			if !strings.HasPrefix(mname, sub) {
				continue
			}
			slog.Ctx(ctx).Debug("new mserver add desc", "probe", mp.Name(), "subject", sub, "metric", mname)
			if ms.config.Verbose {
				ms.descs[mname] = getDescOfMetricVerbose(sub, mname)
			} else {
				ms.descs[mname] = getDescOfMetric(sub, mname)
			}

		}

	}
	// start cache loop
	slog.Ctx(ctx).Debug("new mserver start cache loop")
	ms.loopctrl = make(chan struct{})
	go ms.collectLoop(ctx, cacheUpdateInterval, ms.loopctrl)

	return ms
}

type MServer struct {
	ctx         context.Context
	descs       map[string]*prometheus.Desc
	config      MetricConfig
	metricCache *cache.Cache
	probes      map[string]proto.MetricProbe
	loopctrl    chan struct{}
}

// Close if cache process loop exited, close the metric server will be stuck, check is first
func (s *MServer) Close() {
	if s.loopctrl != nil {
		select {
		case <-s.loopctrl:
			s.loopctrl <- struct{}{}
		default:
		}
	}
}

func (s *MServer) Collect(ch chan<- prometheus.Metric) {
	slog.Ctx(s.ctx).Debug("metric server collect request in", "metric count", len(s.descs))
	for mname, desc := range s.descs {
		data, err := s.collectOnceCache(s.ctx, mname)
		if err != nil || data == nil {
			slog.Ctx(s.ctx).Info("collect metric cache", "err", err, "metric", mname)
			continue
		}
		slog.Ctx(s.ctx).Debug("metric server collect", "metric", mname, "value", data)
		for nsinum, value := range data {
			et, err := nettop2.GetEntityByNetns(int(nsinum))
			if err != nil || et == nil {
				slog.Ctx(s.ctx).Info("collect metric get entity error or nil", "err", err)
				continue
			}
			slog.Ctx(s.ctx).Debug("collect metric", "pod", et.GetPodName(), "netns", nsinum, "metric", mname, "value", value)
			if s.config.Verbose {
				ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(value), nettop2.GetNodeName(), et.GetPodNamespace(), et.GetPodName(), fmt.Sprintf("ns%d", nsinum), et.GetIP(), et.GetAppLabel())
			} else {
				ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(value), nettop2.GetNodeName(), et.GetPodNamespace(), et.GetPodName())
			}
		}
	}
}

// Describe get all description from probe module
func (s *MServer) Describe(ch chan<- *prometheus.Desc) {
	slog.Ctx(s.ctx).Debug("metric server describe request in")
	for m, desc := range s.descs {
		slog.Ctx(s.ctx).Debug("mserver describe", "metric", m)
		ch <- desc
	}
}

func (s *MServer) collectOnceCache(ctx context.Context, metric string) (map[uint32]uint64, error) {
	v, ok := s.metricCache.Get(strings.ToLower(metric))
	if !ok || v == nil {
		slog.Ctx(ctx).Info("collect from cache", "value", v)
		return nil, fmt.Errorf("no cache found for %s", metric)
	}

	vp := v.(map[uint32]uint64)
	if vp == nil {
		slog.Ctx(ctx).Info("collect from cache", "value", v)
		return nil, fmt.Errorf("empty cache found for %s", metric)
	}
	slog.Ctx(ctx).Debug("collect once cache", "metric", metric, "value", vp)
	return vp, nil
}

func (s *MServer) collectLoop(ctx context.Context, interval time.Duration, stopc chan struct{}) {
	slog.Ctx(ctx).Debug("cache loop start", "interval", interval)

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := s.collectWorkerSerial(ctx); err != nil {
				slog.Ctx(ctx).Info("cache loop", "err", err)
				continue
			}
		case <-stopc:
			slog.Ctx(ctx).Info("cache loop stop", "interval", interval)
			close(stopc)
			return
		}
	}
}

// collectWorkerSerial collect metric data in serial
func (s *MServer) collectWorkerSerial(ctx context.Context) error {
	slog.Ctx(s.ctx).Debug("collect worker serial start")
	workdone := make(chan struct{})
	cstart := time.Now()
	ctx, cancelf := context.WithTimeout(ctx, cacheUpdateInterval)
	defer cancelf()

	go func(ctx context.Context, done chan struct{}) {
		for pn, pb := range s.probes {
			start := time.Now()
			// check probe status here
			if !pb.Ready() {
				slog.Ctx(ctx).Info("collect worker not ready", "probe", pn)
				continue
			}
			data, err := pb.Collect(ctx)
			if err != nil {
				slog.Ctx(ctx).Info("collect worker", "err", err, "probe", pn)
				continue
			}
			for mname, mdata := range data {
				slog.Ctx(ctx).Debug("collect worker store", "metric", mname, "value", mdata)
				s.metricCache.Set(mname, mdata, cache.NoExpiration)
			}
			slog.Ctx(ctx).Debug("collect worker finish", "probe", pn)

			CollectLatency.With(prometheus.Labels{"node": nettop2.GetNodeName(), "probe": pn}).Set(float64(time.Since(start).Seconds()))
		}

		done <- struct{}{}
	}(ctx, workdone)

	select {
	case <-ctx.Done():
		slog.Ctx(ctx).Info("collect worker", "time exceeded", time.Since(cstart).Seconds())
		return context.DeadlineExceeded
	case <-workdone:
		slog.Ctx(ctx).Info("collect worker", "finished in", time.Since(cstart).Seconds())
	}

	return nil
}

func getDescOfMetric(mp, mname string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("inspector", "pod", mname),
		fmt.Sprintf("%s %s count in netns/pod", mp, mname),
		[]string{"node", "namespace", "pod"},
		nil,
	)
}

func getDescOfMetricVerbose(mp, mname string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("inspector", "pod", mname),
		fmt.Sprintf("%s %s count in netns/pod", mp, mname),
		[]string{"node", "namespace", "pod", "netns", "ip", "app"},
		nil,
	)
}
