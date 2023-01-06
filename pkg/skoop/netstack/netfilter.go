package netstack

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

type NFHook uint8

type Verdict uint8

const (
	NFHookPreRouting = iota
	NFHookInput
	NFHookForward
	NFHookOutput
	NFHookPostRouting

	VerdictAccept Verdict = 0
	VerdictDrop   Verdict = 1
)

type contextKey string

const (
	ContextIPSetKey  contextKey = "ipset"
	ContextRouterKey contextKey = "router"
)

func (h NFHook) String() string {
	switch h {
	case NFHookPreRouting:
		return "PREROUTING"
	case NFHookInput:
		return "INPUT"
	case NFHookForward:
		return "FORWARD"
	case NFHookOutput:
		return "OUTPUT"
	case NFHookPostRouting:
		return "POSTROUTING"
	default:
		return "INVALID"
	}
}

type Netfilter interface {
	Hook(hook NFHook, packet model.Packet, iif string, oif string) (Verdict, model.Packet, error)
}

type SimulateNetfilterContext struct {
	IPTables IPTables
	IPSet    *IPSetManager
	Router   Router
	IPVS     *IPVS
}

type SimulateNetfilter struct {
	iptables IPTables
	ipvs     *IPVS
	ctx      context.Context
	handles  map[NFHook][]Handle
}

func NewSimulateNetfilter(netfilterContext SimulateNetfilterContext) *SimulateNetfilter {
	ctx := context.TODO()
	ctx = context.WithValue(ctx, ContextIPSetKey, netfilterContext.IPSet)
	ctx = context.WithValue(ctx, ContextRouterKey, netfilterContext.Router)

	nf := &SimulateNetfilter{
		ctx:      ctx,
		iptables: netfilterContext.IPTables,
		ipvs:     netfilterContext.IPVS,
	}
	nf.initHandles()
	return nf
}

type Handle func(hook NFHook, packet *model.Packet, iif, oif string) (Verdict, Trace, error)

var ErrIPTablesUnsupported = errors.New("cannot process iptables")

type IPTableDropError struct {
	Trace Trace
}

func (e *IPTableDropError) Error() string {
	return e.Trace.String()
}

func (nf *SimulateNetfilter) Hook(hook NFHook, packet model.Packet, iif string, oif string) (Verdict, model.Packet, error) {
	if _, ok := nf.iptables.(*emptyIPTables); ok {
		return VerdictAccept, packet, ErrIPTablesUnsupported
	}

	for _, h := range nf.handles[hook] {
		verdict, trace, err := h(hook, &packet, iif, oif)
		if err != nil {
			return verdict, packet, err
		}
		if verdict == VerdictDrop {
			return VerdictDrop, packet, &IPTableDropError{Trace: trace}
		}
	}
	return VerdictAccept, packet, nil
}

func (nf *SimulateNetfilter) doIpt(table string) Handle {
	return func(hook NFHook, packet *model.Packet, iif, oif string) (Verdict, Trace, error) {
		klog.V(4).Infof("hook %d, table %s, input %s", hook, table, packet)
		verdict, trace, err := nf.iptables.TracePacket(nf.ctx, hook, table, packet, iif, oif)
		klog.V(4).Infof("hook %d, table %s, out %s, trace: %s", hook, table, packet, trace)
		return verdict, trace, err
	}
}

func (nf *SimulateNetfilter) initHandles() {
	nf.handles = map[NFHook][]Handle{
		NFHookPreRouting:  {nf.doIpt("raw"), nf.doIpt("mangle"), nf.doIpt("nat")},
		NFHookInput:       {nf.doIpt("mangle"), nf.doIpt("nat"), nf.doIpt("filter")},
		NFHookForward:     {nf.doIpt("mangle"), nf.doIpt("filter")},
		NFHookOutput:      {nf.doIpt("raw"), nf.doIpt("mangle"), nf.doIpt("nat"), nf.doIpt("filter")},
		NFHookPostRouting: {nf.doIpt("mangle"), nf.doIpt("nat")},
	}
}

var _ Netfilter = &SimulateNetfilter{}
