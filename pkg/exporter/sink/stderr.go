package sink

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

type StderrSink struct {
}

func NewStderrSink() *StderrSink {
	return &StderrSink{}
}

func (s StderrSink) Write(event *probe.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed marshal event, err: %w", err)
	}

	fmt.Fprintf(os.Stderr, "event: %s\n", string(data))
	return nil
}

var _ Sink = &StderrSink{}
