package sink

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

type Stderr struct {
}

func NewStderrSink() *Stderr {
	return &Stderr{}
}

func (s Stderr) Write(event *probe.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed marshal event, err: %w", err)
	}

	fmt.Fprintf(os.Stderr, "event: %s", string(data))
	return nil
}

var _ Sink = &Stderr{}
