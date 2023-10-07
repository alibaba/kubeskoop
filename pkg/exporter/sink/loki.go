package sink

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/afiskon/promtail-client/promtail"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

func NewLokiSink(addr string, node string) (*LokiSink, error) {
	url, err := buildURL(addr)
	if err != nil {
		return nil, fmt.Errorf("failed parse addr, not a valild url, err: %w", err)
	}
	log.Infof("create loki client with url %s", url)

	labels := `{instance = "%s",job = "kubeskoop"}`
	conf := promtail.ClientConfig{
		PushURL:            url,
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

func buildURL(addr string) (string, error) {
	if !strings.HasPrefix(addr, "http://") || !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", err
	}

	if u.Path == "" {
		u.Path = "/api/prom/push"
	}

	if u.Port() == "" {
		u.Host = fmt.Sprintf("%s:%s", u.Hostname(), "3100")
	}

	return u.String(), nil
}

type LokiSink struct {
	client promtail.Client
}

func (l *LokiSink) String() string {
	return "loki"
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
