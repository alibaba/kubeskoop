package cmd

import (
	"context"
	"fmt"
	"sync"

	"github.com/samber/lo"

	"strings"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slog"
)

const (
	MetricLabelMeta  = "meta"
	MetricLabelLabel = "label"
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
		mtx:         sync.Mutex{},
		descs:       make(map[string]*prometheus.Desc),
		config:      config,
		probes:      make(map[string]proto.MetricProbe),
		metricCache: cache.New(3*cacheUpdateInterval, 10*cacheUpdateInterval),
		loopctrl:    make(chan struct{}),
	}

	for _, p := range config.Probes {
		mp := probe.GetProbe(p)
		if mp == nil {
			slog.Ctx(ctx).Info("get metric probe nil", "probe", p)
			continue
		}
		ms.probes[p] = mp
		go mp.Start(ctx, proto.ProbeTypeMetrics)
		slog.Ctx(ctx).Debug("new mserver add subject", "subject", p)
	}

	ms.additionalLabels = validateExposeLabels(ms.config.ExposeLabels)
	slog.Default().Debug("metric config", "config", ms.additionalLabels)

	for sub, mp := range ms.probes {
		mnames := mp.GetMetricNames()
		for _, mname := range mnames {
			if !strings.HasPrefix(mname, sub) {
				continue
			}
			slog.Ctx(ctx).Debug("new mserver add desc", "probe", mp.Name(), "subject", sub, "metric", mname)
			if ms.config.Verbose {
				ms.descs[mname] = getDescOfMetricVerbose(sub, mname, ms.additionalLabels)
			} else {
				ms.descs[mname] = getDescOfMetric(sub, mname)
			}

		}
	}
	// start cache loop
	slog.Ctx(ctx).Debug("new mserver start cache loop")
	go ms.collectLoop(ctx, cacheUpdateInterval, ms.loopctrl)

	return ms
}

type MServer struct {
	ctx              context.Context
	mtx              sync.Mutex
	descs            map[string]*prometheus.Desc
	config           MetricConfig
	metricCache      *cache.Cache
	probes           map[string]proto.MetricProbe
	loopctrl         chan struct{}
	additionalLabels []ExposeLabel
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

func (s *MServer) Reload(config MetricConfig) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	enabled := lo.Keys(s.probes)
	toClose, toStart := lo.Difference(enabled, config.Probes)
	slog.Ctx(s.ctx).Info("reload metric probes", "close", toClose, "enable", toStart)

	for _, n := range toClose {
		p, ok := s.probes[n]
		if !ok {
			slog.Ctx(s.ctx).Warn("probe not found in enabled probes, skip.", "probe", n)
			continue
		}

		err := p.Close(proto.ProbeTypeMetrics)
		if err != nil {
			slog.Ctx(s.ctx).Warn("close probe error", "probe", n, "err", err)
			continue
		}
		delete(s.probes, n)
	}

	for _, n := range toStart {
		p := probe.GetProbe(n)
		if p == nil {
			slog.Ctx(s.ctx).Info("get metric probe nil", "probe", n)
			continue
		}
		s.probes[n] = p
		go p.Start(s.ctx, proto.ProbeTypeMetrics)
		slog.Ctx(s.ctx).Debug("new mserver add subject", "subject", n)
	}

	for sub, mp := range s.probes {
		mnames := mp.GetMetricNames()
		for _, mname := range mnames {
			if !strings.HasPrefix(mname, sub) {
				continue
			}
			if s.config.Verbose {
				s.descs[mname] = getDescOfMetricVerbose(sub, mname, s.additionalLabels)
			} else {
				s.descs[mname] = getDescOfMetric(sub, mname)
			}
		}
	}

	s.config = config
	return nil
}

func (s *MServer) Collect(ch chan<- prometheus.Metric) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	slog.Ctx(s.ctx).Debug("metric server collect request in", "metric count", len(s.descs))
	for mname, desc := range s.descs {
		data, err := s.collectOnceCache(s.ctx, mname)
		if err != nil || data == nil {
			slog.Ctx(s.ctx).Info("collect metric cache", "err", err, "metric", mname)
			continue
		}
		slog.Ctx(s.ctx).Debug("metric server collect", "metric", mname, "value", data)
		for nsinum, value := range data {
			et, err := nettop.GetEntityByNetns(int(nsinum))
			if err != nil || et == nil {
				slog.Ctx(s.ctx).Info("collect metric get entity error or nil", "err", err)
				continue
			}
			slog.Ctx(s.ctx).Debug("collect metric", "pod", et.GetPodName(), "netns", nsinum, "metric", mname, "value", value)
			labelValues := []string{nettop.GetNodeName(), et.GetPodNamespace(), et.GetPodName()}
			// for legacy pod labels
			labelValues = append(labelValues, labelValues...)
			if s.config.Verbose {
				if len(s.additionalLabels) > 0 {
					for _, label := range s.additionalLabels {
						switch label.LabelType {
						case "label":
							if value, ok := et.GetLabel(label.Source); ok {
								labelValues = append(labelValues, value)
							} else {
								labelValues = append(labelValues, "")
							}
						case "meta":
							// support ip/netns now
							value, err := et.GetMeta(label.Source)
							if err != nil {
								slog.Default().Info("get meta failed", "meta", label.Source)
								labelValues = append(labelValues, "")
							} else {
								labelValues = append(labelValues, value)
							}
						default:
							// unsupported exposed label will be empty string
							slog.Default().Info("empty label set", "label", label.Source)
							labelValues = append(labelValues, "")
						}
					}
					slog.Default().Info("label values", "label", labelValues)
				}
			}
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(value), labelValues...)
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
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.probes) == 0 {
		return nil
	}
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

			CollectLatency.With(prometheus.Labels{"node": nettop.GetNodeName(), "probe": pn}).Set(float64(time.Since(start).Seconds()))
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

// inspector pod metrics common labels
// {"node", "namespace", "pod"} will override by prometheus default configuration
// refer to https://github.com/alibaba/kubeskoop/issues/77
var defaultMetricLabels = []string{"target_node", "target_namespace", "target_pod", "node", "namespace", "pod"}

func getDescOfMetric(mp, mname string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName("inspector", "pod", mname),
		fmt.Sprintf("%s %s count in netns/pod", mp, mname),
		defaultMetricLabels,
		nil,
	)
}

func getDescOfMetricVerbose(mp, mname string, additionalLabels []ExposeLabel) *prometheus.Desc {
	labels := defaultMetricLabels
	if len(additionalLabels) > 0 {
		for _, label := range additionalLabels {
			slog.Info("build metric description", "additional label", label)
			labels = append(labels, label.Replace)
		}
	}
	return prometheus.NewDesc(
		prometheus.BuildFQName("inspector", "pod", mname),
		fmt.Sprintf("%s %s count in netns/pod", mp, mname),
		labels,
		nil,
	)
}

func validateExposeLabels(labels []ExposeLabel) []ExposeLabel {
	res := []ExposeLabel{}
	for _, label := range labels {
		if label.LabelType != MetricLabelLabel && label.LabelType != MetricLabelMeta {
			continue
		}

		if label.Source == "" {
			continue
		}

		if label.Replace == "" {
			label.Replace = label.Source
		}

		res = append(res, label)
	}

	return res
}
