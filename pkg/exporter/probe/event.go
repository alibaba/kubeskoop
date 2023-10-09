package probe

import (
	"fmt"
	"reflect"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"golang.org/x/exp/slog"
)

var (
	availableEventProbe = make(map[string]*EventProbeCreator)
)

// type EventProbeCreator func(sink chan<- *Event, args map[string]interface{}) (EventProbe, error)
type EventProbeCreator struct {
	f reflect.Value
	s *reflect.Type
}

func NewEventProbeCreator(creator interface{}) (*EventProbeCreator, error) {
	t := reflect.TypeOf(creator)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("metric probe creator %#v is not a func", creator)
	}

	err := validateProbeCreatorReturnValue[EventProbe](reflect.TypeOf(creator))
	if err != nil {
		return nil, err
	}

	if t.NumIn() != 1 && t.NumIn() != 2 {
		return nil, fmt.Errorf("input parameter count of creator should be either 2 or 3")
	}

	ct := t.In(0)
	et := reflect.TypeOf((*Event)(nil))
	if ct.Kind() != reflect.Chan || ct.ChanDir() != reflect.SendDir || ct.Elem() != et {
		return nil, fmt.Errorf("first input parameter should be a send channel of *Event")
	}

	ret := &EventProbeCreator{
		f: reflect.ValueOf(creator),
	}

	if t.NumIn() == 2 {
		st := t.In(1)
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

func (e *EventProbeCreator) Call(sink chan<- *Event, args map[string]interface{}) (EventProbe, error) {
	in := []reflect.Value{
		reflect.ValueOf(sink),
	}
	if e.s != nil {
		s, err := createStructFromTypeWithArgs(*e.s, args)
		if err != nil {
			return nil, err
		}
		in = append(in, s)
	}

	result := e.f.Call(in)
	// return parameter count and type has been checked in NewEventProbeCreator
	ret := result[0].Interface().(EventProbe)
	err := result[1].Interface()
	if err == nil {
		return ret, nil
	}
	return ret, err.(error)
}

func MustRegisterEventProbe(name string, creator interface{}) {
	if _, ok := availableEventProbe[name]; ok {
		panic(fmt.Errorf("duplicated event probe %s", name))
	}

	c, err := NewEventProbeCreator(creator)
	if err != nil {
		panic(fmt.Errorf("error register event probe %s: %s", name, err))
	}

	availableEventProbe[name] = c
}

func NewEventProbe(name string, simpleProbe SimpleProbe) EventProbe {
	return NewProbe(name, simpleProbe)
}

func CreateEventProbe(name string, sink chan<- *Event, args map[string]interface{}) (EventProbe, error) {
	creator, ok := availableEventProbe[name]
	if !ok {
		return nil, fmt.Errorf("undefined probe %s", name)
	}

	return creator.Call(sink, args)
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
