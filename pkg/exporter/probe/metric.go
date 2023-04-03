package probe

import (
	"context"
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procio"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procnetdev"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procnetstat"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procsnmp"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procsock"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/procsoftnet"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/proctcpsummary"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracekernel"
	tracenetif "github.com/alibaba/kubeskoop/pkg/exporter/probe/tracenetiftxlatency"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracenetsoftirq"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracepacketloss"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracesocketlatency"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracevirtcmdlat"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
)

var (
	availmprobes map[string]proto.MetricProbe
)

func init() {
	availmprobes = map[string]proto.MetricProbe{}

	availmprobes["tcp"] = procsnmp.GetProbe()
	availmprobes["udp"] = procsnmp.GetProbe()
	availmprobes["ip"] = procsnmp.GetProbe()
	availmprobes["netdev"] = procnetdev.GetProbe()
	availmprobes["softnet"] = procsoftnet.GetProbe()
	availmprobes["sock"] = procsock.GetProbe()
	availmprobes["io"] = procio.GetProbe()
	availmprobes["tcpext"] = procnetstat.GetProbe()
	availmprobes["socketlatency"] = tracesocketlatency.GetProbe()
	availmprobes["packetloss"] = tracepacketloss.GetProbe()
	availmprobes["net_softirq"] = tracenetsoftirq.GetProbe()
	availmprobes["netiftxlat"] = tracenetif.GetProbe()
	availmprobes["kernellatency"] = tracekernel.GetProbe()
	availmprobes["tcpsummary"] = proctcpsummary.GetProbe()
	availmprobes["virtcmdlatency"] = tracevirtcmdlat.GetProbe()
}

func ListMetricProbes(_ context.Context, _ bool) (probelist []string) {
	for k := range availmprobes {
		probelist = append(probelist, k)
	}
	return
}

func ListMetrics() map[string][]string {
	mm := make(map[string][]string)
	for p, pb := range availmprobes {
		if pb != nil {
			// for multi metric of one probe,filter with prefix
			mnames := pb.GetMetricNames()
			mm[p] = append(mm[p], mnames...)
		}
	}
	return mm
}

func GetProbe(subject string) proto.MetricProbe {
	if p, ok := availmprobes[subject]; ok {
		return p
	}

	return nil
}

// CollectOnce collect from probe directly for test
func CollectOnce(ctx context.Context, subject string) (map[string]map[uint32]uint64, error) {
	return collectOnce(ctx, subject)
}

func collectOnce(ctx context.Context, subject string) (map[string]map[uint32]uint64, error) {
	probe, ok := availmprobes[subject]
	if !ok {
		return nil, fmt.Errorf("no probe found of %s", subject)
	}

	return probe.Collect(ctx)
}
