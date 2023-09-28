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
	}
	return nil, fmt.Errorf("unknown sink type %s", name)
}
