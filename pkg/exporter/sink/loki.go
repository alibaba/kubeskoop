package sink

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/afiskon/promtail-client/promtail"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

func NewLokiSink(addr string, node string) (*LokiSink, error) {
	labels := `{instance = "%s",job = "inspector"}`
	conf := promtail.ClientConfig{
		PushURL:            addr,
		Labels:             fmt.Sprintf(labels, node),
		BatchWait:          5 * time.Second,
		BatchEntriesNumber: 10000,
		SendLevel:          promtail.DEBUG,
		PrintLevel:         promtail.DEBUG,
	}
	client, err := promtail.NewClientProto(conf)
	if err != nil {
		return nil, fmt.Errorf("failed create loki client, err: %s", err)
	}
	return &LokiSink{
		client: client,
	}, nil
}

type LokiSink struct {
	client promtail.Client
}

func (l *LokiSink) Write(event *probe.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed marshal event, err: %w", err)
	}

	l.client.Infof(string(data))
	return nil
}

var _ Sink = &LokiSink{}
