package probe

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var legacyMetricsLabels = []string{"target_node", "target_namespace", "target_pod", "node", "namespace", "pod"}
var newMetricsLabels = []string{"k8s_node", "k8s_namespace", "k8s_pod"}

type legacyBatchMetrics struct {
	module     string
	collector  LegacyCollector
	descs      map[string]*prometheus.Desc
	underscore bool
}

func legacyMetricsName(module string, name string, underscore bool) string {
	if underscore {
		return fmt.Sprintf("%s_pod_%s_%s", LegacyMetricsNamespace, module, name)
	}
	return fmt.Sprintf("%s_pod_%s%s", LegacyMetricsNamespace, module, name)
}
func newMetricsName(module, name string) string {
	return prometheus.BuildFQName(MetricsNamespace, module, name)
}

type LegacyCollector func() (map[string]map[uint32]uint64, error)

func NewLegacyBatchMetrics(module string, metrics []string, collector LegacyCollector) prometheus.Collector {
	return newLegacyBatchMetrics(module, false, metrics, collector)
}

func newLegacyBatchMetrics(module string, underscore bool, metrics []string, collector LegacyCollector) prometheus.Collector {
	descs := make(map[string]*prometheus.Desc)
	for _, m := range metrics {
		legacyName := legacyMetricsName(module, m, underscore)
		newName := newMetricsName(module, m)
		descs[legacyName] = prometheus.NewDesc(legacyName, "", legacyMetricsLabels, nil)
		descs[newName] = prometheus.NewDesc(newName, "", newMetricsLabels, nil)
	}
	return &legacyBatchMetrics{
		module:     module,
		collector:  collector,
		descs:      descs,
		underscore: underscore,
	}
}

func NewLegacyBatchMetricsWithUnderscore(module string, metrics []string, collector LegacyCollector) prometheus.Collector {
	return newLegacyBatchMetrics(module, true, metrics, collector)
}

func (l *legacyBatchMetrics) Describe(descs chan<- *prometheus.Desc) {
	for _, desc := range l.descs {
		descs <- desc
	}
}

func (l *legacyBatchMetrics) Collect(metrics chan<- prometheus.Metric) {
	log.Debugf("collect data from %s", l.module)
	data, err := l.collector()
	if err != nil {
		log.Errorf("%s failed collect data, err: %v", l.module, err)
		return
	}

	emit := func(name string, labelValues []string, value float64) {
		desc, ok := l.descs[name]
		if !ok {
			return
		}

		metrics <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(value), labelValues...)
	}

	for key, namespaceData := range data {
		for nsinum, value := range namespaceData {
			et, err := nettop.GetEntityByNetns(int(nsinum))
			if err != nil || et == nil {
				continue
			}
			labelValues := []string{nettop.GetNodeName(), et.GetPodNamespace(), et.GetPodName()}
			// for legacy pod labels
			emit(newMetricsName(l.module, key), labelValues, float64(value))

			labelValues = append(labelValues, labelValues...)
			emit(legacyMetricsName(l.module, key, l.underscore), labelValues, float64(value))
		}
	}
}

func LagacyEventLabels(netns uint32) []Label {
	et, err := nettop.GetEntityByNetns(int(netns))
	if err != nil {
		log.Infof("nettop get entity failed, netns: %d, err: %v", netns, err)
		return nil
	}
	return []Label{
		{Name: "pod", Value: et.GetPodName()},
		{Name: "namespace", Value: et.GetPodNamespace()},
		{Name: "node", Value: nettop.GetNodeName()},
	}
}

func CopyLegacyMetricsMap(m map[string]map[uint32]uint64) map[string]map[uint32]uint64 {
	ret := make(map[string]map[uint32]uint64)
	for key, nsMap := range m {
		ret[key] = make(map[uint32]uint64)
		for ns, data := range nsMap {
			ret[key][ns] = data
		}
	}
	return ret
}
