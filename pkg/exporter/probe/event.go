package probe

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"golang.org/x/exp/slog"
)

var (
	availableEventProbe = make(map[string]EventProbeCreator)
)

type EventProbeCreator func(sink chan<- *Event, args map[string]interface{}) (EventProbe, error)

func MustRegisterEventProbe(name string, creator EventProbeCreator) {
	if _, ok := availableEventProbe[name]; ok {
		panic(fmt.Errorf("duplicated event probe %s", name))
	}

	availableEventProbe[name] = creator
}

func NewEventProbe(name string, simpleProbe SimpleProbe) EventProbe {
	return NewProbe(name, simpleProbe)
}

func CreateEventProbe(name string, sink chan<- *Event, _ interface{}) (EventProbe, error) {
	creator, ok := availableEventProbe[name]
	if !ok {
		return nil, fmt.Errorf("undefined probe %s", name)
	}

	//TODO reflect creator's arguments
	return creator(sink, nil)
}

func ListEventProbes() []string {
	var ret []string
	for key := range availableEventProbe {
		ret = append(ret, key)
	}
	return ret
}

func EventMetaByNetNS(netns int) []Label {
	et, err := nettop.GetEntityByNetns(netns)
	if err != nil {
		slog.Info("nettop get entity", "err", err, "netns", netns)
		return nil
	}
	return []Label{
		{Name: "pod", Value: et.GetPodName()},
		{Name: "namespace", Value: et.GetPodNamespace()},
		{Name: "node", Value: nettop.GetNodeName()},
	}
}
