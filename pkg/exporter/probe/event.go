package probe

import (
	"fmt"
	"reflect"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"golang.org/x/exp/slog"
)

var (
	availableEventProbe = make(map[string]*eventProbeCreator)
)

type eventProbeCreator struct {
	f reflect.Value
	s *reflect.Type
}

func newEventProbeCreator(creator interface{}) (*eventProbeCreator, error) {
	t := reflect.TypeOf(creator)
	err := validateProbeCreatorReturnValue[EventProbe](t)
	if err != nil {
		return nil, err
	}

	if t.NumIn() != 1 && t.NumIn() != 2 {
		return nil, fmt.Errorf("input parameter count of creator should be either 1 or 2")
	}

	ct := t.In(0)
	et := reflect.TypeOf((*Event)(nil))
	if ct.Kind() != reflect.Chan || ct.ChanDir() != reflect.SendDir || ct.Elem() != et {
		return nil, fmt.Errorf("first input parameter type should be chan<- *Event")
	}

	ret := &eventProbeCreator{
		f: reflect.ValueOf(creator),
	}

	if t.NumIn() == 2 {
		st := t.In(1)
		if err := validateParamTypeMapOrStruct(st); err != nil {
			return nil, err
		}
		ret.s = &st
	}

	return ret, nil
}

func (e *eventProbeCreator) Call(sink chan<- *Event, args map[string]interface{}) (ep EventProbe, err error) {
	in := []reflect.Value{
		reflect.ValueOf(sink),
	}
	if e.s != nil {
		s, e := createStructFromTypeWithArgs(*e.s, args)
		if e != nil {
			return nil, e
		}
		in = append(in, s)
	}

	result := e.f.Call(in)
	// return parameter count and type has been checked in newEventProbeCreator

	if result[0].Interface() == nil {
		ep = nil
	} else {
		ep = result[0].Interface().(EventProbe)
	}

	if result[1].Interface() == nil {
		err = nil
	} else {
		err = result[1].Interface().(error)
	}

	return
}

// MustRegisterEventProbe registers the event probe by given name and creator.
// The creator is a function that creates EventProbe. Return values of the creator
// must be (EventProbe, error). The creator can accept one parameter
// of type chan<- *Event, or struct/map as an extra parameter.
// When the creator specifies the extra parameter, the configuration of the probe in the configuration file
// will be passed to the creator when the probe is created. For example:
//
// The creator accepts no extra args.
//
//	func eventProbeCreator(sink chan<- *Event) (EventProbe, error)
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
//		func eventProbeCreator(sink chan<- *Event, args probeArgs) (EventProbe, error)
//
// The creator can also use a map with string keys as parameters.
// However, if you use a type other than interface{} as the value type, errors may occur
// during the configuration parsing process.
//
//	func metricsProbeCreator(sink chan<- *Event, args map[string]string) (EventProbe, error)
//	func metricsProbeCreator(sink chan<- *Event, args map[string]interface{} (EventProbe, error)
func MustRegisterEventProbe(name string, creator interface{}) {
	if _, ok := availableEventProbe[name]; ok {
		panic(fmt.Errorf("duplicated event probe %s", name))
	}

	c, err := newEventProbeCreator(creator)
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
