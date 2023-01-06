package lokiwrapper

import (
	"context"
	"fmt"
	"net"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"time"

	"github.com/afiskon/promtail-client/promtail"
	"golang.org/x/exp/slog"
)

var (
	eventTmpl = "type=%s pod=%s namespace=%s %s"
)

type LokiIngester struct {
	promtail.Client
	addr     string
	datach   chan proto.RawEvent
	instance string
}

func (i *LokiIngester) Name() string {
	return fmt.Sprintf("loki_%s", i.addr)
}

func (i *LokiIngester) Watch(ctx context.Context) {
	for evt := range i.datach {
		et, err := nettop.GetEntityByNetns(int(evt.Netns))
		if err != nil {
			slog.Ctx(ctx).Warn("watch get entity", "err", err, "netns", evt.Netns, "client", i.Name())
			continue
		}

		evtStr := fmt.Sprintf(eventTmpl, evt.EventType, et.GetPodName(), et.GetPodNamespace(), evt.EventBody)
		i.Client.Infof(evtStr)
	}
}

func NewIngester(addr string, node string, datach chan proto.RawEvent) (*LokiIngester, error) {
	ingester := &LokiIngester{
		datach: datach,
	}

	if net.ParseIP(addr) == nil {
		return ingester, fmt.Errorf("invalid loki remote address %s", addr)
	}
	ingester.addr = fmt.Sprintf("http://%s:3100/api/prom/push", addr)
	ingester.instance = node
	labels := `{instance = "%s",job = "inspector"}`
	conf := promtail.ClientConfig{
		PushURL:            ingester.addr,
		Labels:             fmt.Sprintf(labels, ingester.instance),
		BatchWait:          5 * time.Second,
		BatchEntriesNumber: 10000,
		SendLevel:          promtail.DEBUG,
		PrintLevel:         promtail.DEBUG,
	}
	client, err := promtail.NewClientProto(conf)
	if err != nil {
		return ingester, fmt.Errorf("register ingester clien %s with %s", addr, err.Error())
	}
	ingester.Client = client

	return ingester, nil
}
