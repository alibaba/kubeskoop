package sink

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

const (
	Stderr = "stderr"
	File   = "file"
	Loki   = "loki"
    Flame  = "flame"
)

type Sink interface {
	Write(event *probe.Event) error
}

func CreateSink(name string, args interface{}) (Sink, error) {
	//TODO create with register and reflect
	argsMap, _ := args.(map[string]interface{})

	switch name {
	case Stderr:
		return NewStderrSink(), nil
	case Loki:
		addr := argsMap["addr"].(string)
		return NewLokiSink(addr, nettop.GetNodeName())
	case File:
		path := argsMap["path"].(string)
		return NewFileSink(path)
    case Flame:
        return NewFlameSink(), nil
	}
	return nil, fmt.Errorf("unknown sink type %s", name)
}
