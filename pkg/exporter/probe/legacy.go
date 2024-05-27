package probe

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var StandardMetricsLabels = []string{"k8s_node", "k8s_namespace", "k8s_pod"}
var TupleMetricsLabels = []string{"protocol", "src", "src_type", "src_node", "src_namespace", "src_pod", "dst", "dst_type", "dst_node", "dst_namespace", "dst_pod", "sport", "dport"}
var AdditionalLabelValueExpr []string

func BuildStandardMetricsLabelValues(entity *nettop.Entity) []string {
	metaPodLabels := []string{nettop.GetNodeName(), entity.GetPodNamespace(), entity.GetPodName()}
	return append(metaPodLabels, BuildAdditionalLabelsValues(entity.GetLabels())...)
}

func InitAdditionalLabels(additionalLabels []string) error {
	if len(additionalLabels) == 0 {
		return nil
	}

	//append to StandardMetricsLabels and AdditionalLabelValueExpr
	for additionalKV := range additionalLabels {
		labelKVPair := strings.Split(additionalLabels[additionalKV], "=")
		StandardMetricsLabels = append(StandardMetricsLabels, strings.TrimSpace(labelKVPair[0]))
		AdditionalLabelValueExpr = append(AdditionalLabelValueExpr, strings.TrimSpace(labelKVPair[1]))
	}

	return nil
}

func BuildAdditionalLabelsValues(podLabels map[string]string) []string {
	if len(AdditionalLabelValueExpr) == 0 {
		return []string{}
	}

	var values []string

	var replaceAllStringSubmatchFunc = func(re *regexp.Regexp, str string, repl func([]string) string) string {
		result := ""
		lastIndex := 0

		for _, v := range re.FindAllSubmatchIndex([]byte(str), -1) {
			var groups []string
			for i := 0; i < len(v); i += 2 {
				groups = append(groups, str[v[i]:v[i+1]])
			}

			result += str[lastIndex:v[0]] + repl(groups)
			lastIndex = v[1]
		}

		return result + str[lastIndex:]
	}

	for _, labelValueExpr := range AdditionalLabelValueExpr {
		podLabelValue := replaceAllStringSubmatchFunc(regexp.MustCompile(`\${labels:(.*?)}`), labelValueExpr, func(groups []string) string {
			if podLabelValue, ok := podLabels[groups[1]]; ok {
				return podLabelValue
			}
			return ""
		})
		values = append(values, podLabelValue)
	}

	return values
}

type legacyBatchMetrics struct {
	module    string
	collector LegacyCollector
	descs     map[string]*prometheus.Desc
}

func newMetricsName(module, name string) string {
	return prometheus.BuildFQName(MetricsNamespace, module, name)
}

type LegacyCollector func() (map[string]map[uint32]uint64, error)

func NewLegacyBatchMetrics(module string, metrics []string, collector LegacyCollector) prometheus.Collector {
	return newLegacyBatchMetrics(module, metrics, collector)
}

func newLegacyBatchMetrics(module string, metrics []string, collector LegacyCollector) prometheus.Collector {
	descs := make(map[string]*prometheus.Desc)
	for _, m := range metrics {
		newName := newMetricsName(module, m)
		descs[newName] = prometheus.NewDesc(newName, "", StandardMetricsLabels, nil)
	}
	return &legacyBatchMetrics{
		module:    module,
		collector: collector,
		descs:     descs,
	}
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
			labelValues := BuildStandardMetricsLabelValues(et)
			emit(newMetricsName(l.module, key), labelValues, float64(value))
		}
	}
}

func LegacyEventLabels(netns uint32) []Label {
	et, err := nettop.GetEntityByNetns(int(netns))
	if err != nil || et == nil {
		log.Infof("nettop get entity failed, netns: %d, err: %v", netns, err)
		return nil
	}
	return []Label{
		{Name: "pod", Value: et.GetPodName()},
		{Name: "namespace", Value: et.GetPodNamespace()},
		{Name: "node", Value: nettop.GetNodeName()},
	}
}

func BuildTupleMetricsLabels(tuple *Tuple) []string {
	ipInfo := func(ip string) []string {
		info := nettop.GetIPInfo(ip)
		if info == nil {
			return []string{"unknown", "", "", ""}
		}

		switch info.Type {
		case nettop.IPTypeNode:
			return []string{"node", info.NodeName, "", ""}
		case nettop.IPTypePod:
			return []string{"pod", "", info.PodNamespace, info.PodName}
		default:
			log.Warningf("unknown ip type %s for %s", ip, info.Type)
		}
		return []string{"unknown", "", "", ""}
	}

	labels := []string{bpfutil.GetProtoStr(tuple.Protocol)}
	labels = append(labels, tuple.Src)
	labels = append(labels, ipInfo(tuple.Src)...)

	labels = append(labels, tuple.Dst)
	labels = append(labels, ipInfo(tuple.Dst)...)
	labels = append(labels, fmt.Sprintf("%d", tuple.Sport))
	labels = append(labels, fmt.Sprintf("%d", tuple.Dport))
	return labels
}

func BuildTupleEventLabels(tuple *Tuple) []Label {
	ipInfo := func(prefix, ip string) (ret []Label) {
		var values [4]string

		defer func() {
			ret = []Label{
				{Name: prefix + "_type", Value: values[0]},
				{Name: prefix + "_node", Value: values[1]},
				{Name: prefix + "_namespace", Value: values[2]},
				{Name: prefix + "_pod", Value: values[3]},
			}

		}()

		info := nettop.GetIPInfo(ip)
		if info == nil {
			values = [...]string{"unknown", "", "", ""}
			return
		}

		switch info.Type {
		case nettop.IPTypeNode:
			values = [...]string{"node", info.NodeName, "", ""}
		case nettop.IPTypePod:
			values = [...]string{"pod", "", info.PodNamespace, info.PodName}
		default:
			log.Warningf("unknown ip type %s for %s", ip, info.Type)
		}
		values = [...]string{"unknown", "", "", ""}
		return
	}

	labels := []Label{
		{Name: "protocol", Value: bpfutil.GetProtoStr(tuple.Protocol)},
	}
	labels = append(labels, Label{
		Name: "src", Value: tuple.Src,
	})

	labels = append(labels, ipInfo("src", tuple.Src)...)

	labels = append(labels, Label{
		Name: "dst", Value: tuple.Dst,
	})
	labels = append(labels, ipInfo("dst", tuple.Dst)...)
	labels = append(labels, Label{
		Name: "sport", Value: fmt.Sprintf("%d", tuple.Sport),
	})

	labels = append(labels, Label{
		Name: "dport", Value: fmt.Sprintf("%d", tuple.Dport),
	})

	return labels
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
