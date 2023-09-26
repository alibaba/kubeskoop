package sink

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

func NewFileSink(path string) (*FileSink, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed open file %s, err: %w", path, err)
	}

	return &FileSink{
		file: file,
	}, nil
}

type FileSink struct {
	file *os.File
}

func (f *FileSink) Write(event *probe.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed marshal event, err: %w", err)
	}
	_, err = f.file.Write(data)
	f.file.Write([]byte{0x0a})

	if err != nil {
		return fmt.Errorf("failed sink event to file %s, err: %w", f.file.Name(), err)
	}
	return nil

}

var _ Sink = &FileSink{}
