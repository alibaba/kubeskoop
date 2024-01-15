package sink

import (
	"encoding/json"
	"fmt"

	lokiwrapper "github.com/alibaba/kubeskoop/pkg/exporter/loki"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

func NewLokiSink(addr string, node string) (*LokiSink, error) {
	client, err := lokiwrapper.NewLokiIngester(addr, node)
	if err != nil {
		return nil, fmt.Errorf("failed create loki client, err: %s", err)
	}
	return &LokiSink{
		client: client,
	}, nil
}

type LokiSink struct {
	client *lokiwrapper.Ingester
}

func (l *LokiSink) String() string {
	return "loki"
}

func (l *LokiSink) Write(event *probe.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed marshal event, err: %w", err)
	}

	l.client.Send(string(data))
	return nil
}

var _ Sink = &LokiSink{}
