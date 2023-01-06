package probe

import (
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracebiolatency"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracekernel"
	tracenetif "github.com/alibaba/kubeskoop/pkg/exporter/probe/tracenetiftxlatency"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracenetsoftirq"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracepacketloss"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracesocketlatency"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracetcpreset"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe/tracevirtcmdlat"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
)

var (
	availeprobes map[string]proto.EventProbe
)

func init() {

	availeprobes = map[string]proto.EventProbe{}

	availeprobes["netiftxlatency"] = tracenetif.GetProbe()
	availeprobes["biolatency"] = tracebiolatency.GetProbe()
	availeprobes["net_softirq"] = tracenetsoftirq.GetProbe()
	availeprobes["tcpreset"] = tracetcpreset.GetProbe()
	availeprobes["kernellatency"] = tracekernel.GetProbe()
	availeprobes["packetloss"] = tracepacketloss.GetProbe()
	availeprobes["socketlatency"] = tracesocketlatency.GetProbe()
	availeprobes["virtcmdlatency"] = tracevirtcmdlat.GetProbe()
}

func GetEventProbe(subject string) proto.EventProbe {
	if p, ok := availeprobes[subject]; ok {
		return p
	}

	return nil
}

func ListEvents() map[string][]string {
	em := make(map[string][]string)

	for p, ep := range availeprobes {
		enames := ep.GetEventNames()
		em[p] = append(em[p], enames...)
	}

	return em
}
