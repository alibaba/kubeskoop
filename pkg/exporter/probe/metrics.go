package probe

import (
	"errors"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
)

const LegacyMetricsNamespace = "inspector"
const MetricsNamespace = "kubeskoop"

var (
	availableMetricsProbes = make(map[string]*MetricsProbeCreator)
	ErrUndeclaredMetrics   = errors.New("undeclared metrics")
)

// type MetricsProbeCreator func(args map[string]interface{}) (MetricsProbe, error)
type MetricsProbeCreator struct {
	f reflect.Value
	s *reflect.Type
}

func NewMetricProbeCreator(creator interface{}) (*MetricsProbeCreator, error) {
	t := reflect.TypeOf(creator)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("metric probe creator %#v is not a func", creator)
	}

	err := validateProbeCreatorReturnValue[MetricsProbe](reflect.TypeOf(creator))
	if err != nil {
		return nil, err
	}

	if t.NumIn() > 1 {
		return nil, fmt.Errorf("input parameter count of creator should be either 0 or 1")
	}

	ret := &MetricsProbeCreator{
		f: reflect.ValueOf(creator),
	}

	if t.NumIn() == 1 {
		st := t.In(0)
		if st.Kind() != reflect.Struct && st.Kind() != reflect.Map {
			return nil, fmt.Errorf("input parameter should be struct, but %s", st.Kind())
		}
		if st.Kind() == reflect.Map && st.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key type of input parameter should be string")
		}
		ret.s = &st
	}

	return ret, nil
}

func (m *MetricsProbeCreator) Call(args map[string]interface{}) (MetricsProbe, error) {
	var in []reflect.Value
	if m.s != nil {
		s, err := createStructFromTypeWithArgs(*m.s, args)
		if err != nil {
			return nil, err
		}
		in = append(in, s)
	}

	result := m.f.Call(in)
	// return parameter count and type has been checked in NewMetricProbeCreator
	ret := result[0].Interface().(MetricsProbe)
	err := result[1].Interface()
	if err == nil {
		return ret, nil
	}
	return ret, err.(error)
}

// MustRegisterMetricsProbe registers the metrics probe by given name and creator.
// The creator is a function that creates MetricProbe. Return values of the creator
// must be (MetricsProbe, error). The creator can accept no parameter, or struct/map as a parameter.
// When the creator specifies the parameter, the configuration of the probe in the configuration file
// will be passed to the creator when the probe is created. For example:
//
// The creator accepts no extra args.
//
//	func metricsProbeCreator() (MetricsProbe, error)
//
// The creator accepts struct "probeArgs" as args. Names of struct fields are case-insensitive.
//
//		// Config in yaml
//		args:
//	      argA: test
//		  argB: 20
//		  argC:
//		    - a
//		// Struct definition
//		type probeArgs struct {
//		  ArgA string
//		  ArgB int
//		  ArgC []string
//		}
//		// The creator function:
//		func metricsProbeCreator(args probeArgs) (MetricsProbe, error)
//
// The creator can also use a map with string keys as parameters.
// However, if you use a type other than interface{} as the value type, errors may occur
// during the configuration parsing process.
//
//	func metricsProbeCreator(args map[string]string) (MetricsProbe, error)
//	func metricsProbeCreator(args map[string]interface{} (MetricsProbe, error)
func MustRegisterMetricsProbe(name string, creator interface{}) {
	if _, ok := availableMetricsProbes[name]; ok {
		panic(fmt.Errorf("duplicated metric probe %s", name))
	}

	c, err := NewMetricProbeCreator(creator)
	if err != nil {
		panic(fmt.Errorf("error register metric probe %s: %s", name, err))
	}

	availableMetricsProbes[name] = c
}

func CreateMetricsProbe(name string, args map[string]interface{}) (MetricsProbe, error) {
	creator, ok := availableMetricsProbes[name]
	if !ok {
		return nil, fmt.Errorf("undefined probe %s", name)
	}

	return creator.Call(args)
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
		m, err := prometheus.NewConstMetric(info.desc, info.valueType, val, labels...)
		if err != nil {
			log.Errorf("%s failed create metrics, err: %v", b.name, err)
			return
		}
		metrics <- m
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
