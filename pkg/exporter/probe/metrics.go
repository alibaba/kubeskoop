package probe

import (
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
)

const LegacyMetricsNamespace = "inspector"
const MetricsNamespace = "kubeskoop"

var (
	availableMetricsProbes = make(map[string]MetricsProbeCreator)
	ErrUndeclaredMetrics   = errors.New("undeclared metrics")
)

type MetricsProbeCreator func(args map[string]interface{}) (MetricsProbe, error)

func MustRegisterMetricsProbe(name string, creator MetricsProbeCreator) {
	if _, ok := availableMetricsProbes[name]; ok {
		panic(fmt.Errorf("duplicated event probe %s", name))
	}

	availableMetricsProbes[name] = creator
}

func CreateMetricsProbe(name string, _ interface{}) (MetricsProbe, error) {
	creator, ok := availableMetricsProbes[name]
	if !ok {
		return nil, fmt.Errorf("undefined probe %s", name)
	}

	//TODO reflect creator's arguments
	return creator(nil)
}

func ListMetricsProbes() []string {
	var ret []string
	for key := range availableMetricsProbes {
		ret = append(ret, key)
	}
	return ret
}

type Emit func(name string, labels []string, val float64)

type Collector func(emit Emit) error

type SingleMetricsOpts struct {
	Name           string
	Help           string
	ConstLabels    map[string]string
	VariableLabels []string
	ValueType      prometheus.ValueType
}

type BatchMetricsOpts struct {
	Namespace         string
	Subsystem         string
	ConstLabels       map[string]string
	VariableLabels    []string
	SingleMetricsOpts []SingleMetricsOpts
}

type metricsInfo struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

type BatchMetrics struct {
	name           string
	infoMap        map[string]*metricsInfo
	ProbeCollector Collector
}

func NewBatchMetrics(opts BatchMetricsOpts, probeCollector Collector) *BatchMetrics {
	m := make(map[string]*metricsInfo)
	for _, metrics := range opts.SingleMetricsOpts {
		constLabels, variableLables := mergeLabels(opts, metrics)
		desc := prometheus.NewDesc(
			prometheus.BuildFQName(opts.Namespace, opts.Subsystem, metrics.Name),
			metrics.Help,
			variableLables,
			constLabels,
		)

		m[metrics.Name] = &metricsInfo{
			desc:      desc,
			valueType: metrics.ValueType,
		}
	}

	return &BatchMetrics{
		name:           fmt.Sprintf("%s_%s", opts.Namespace, opts.Subsystem),
		infoMap:        m,
		ProbeCollector: probeCollector,
	}
}

func (b *BatchMetrics) Describe(descs chan<- *prometheus.Desc) {
	for _, info := range b.infoMap {
		descs <- info.desc
	}
}

func (b *BatchMetrics) Collect(metrics chan<- prometheus.Metric) {
	emit := func(name string, labels []string, val float64) {
		info, ok := b.infoMap[name]
		if !ok {
			log.Errorf("%s undeclared metrics %s", b.name, name)
			return
		}
		metrics <- prometheus.MustNewConstMetric(info.desc, info.valueType, val, labels...)
	}

	err := b.ProbeCollector(emit)
	if err != nil {
		log.Errorf("%s error collect, err: %v", b.name, err)
		return
	}
}

func mergeLabels(opts BatchMetricsOpts, metrics SingleMetricsOpts) (map[string]string, []string) {
	constLabels := mergeMap(opts.ConstLabels, metrics.ConstLabels)
	variableLabels := mergeArray(opts.VariableLabels, metrics.VariableLabels)

	return constLabels, variableLabels
}

func mergeArray(labels []string, labels2 []string) []string {
	m := make(map[string]bool)
	for _, s := range labels {
		m[s] = true
	}

	for _, s := range labels2 {
		if _, ok := m[s]; ok {
			//to avoid duplicated label
			panic(fmt.Sprintf("metric label %s already declared in BatchMetricsOpts", s))
		}
	}

	var ret []string
	for k := range m {
		ret = append(ret, k)
	}

	return ret
}

// if a key exists in both maps, value in labels2 will be keep
func mergeMap(labels map[string]string, labels2 map[string]string) map[string]string {
	ret := make(map[string]string)
	maps.Copy(ret, labels)
	maps.Copy(ret, labels2)
	return ret
}

type combinedMetricsProbe struct {
	Probe
	prometheus.Collector
}

func NewMetricsProbe(name string, simpleProbe SimpleProbe, collector prometheus.Collector) MetricsProbe {
	return &combinedMetricsProbe{
		Probe:     NewProbe(name, simpleProbe),
		Collector: collector,
	}
}

var _ prometheus.Collector = &BatchMetrics{}
