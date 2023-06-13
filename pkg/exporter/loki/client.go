package lokiwrapper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/loki/logproto"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	inspproto "github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/golang/snappy"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/proto"
	timestamp "google.golang.org/protobuf/types/known/timestamppb"
)

type LogLevel int

const (
	LogEntriesChanSize = 5000
)

var (
	eventTmpl = "type=%s pod=%s namespace=%s %s"

	BatchEntriesNumber = 10000
	BatchWait          = 5 * time.Second
)

type httpClient struct {
	parent http.Client
}

func NewLokiIngester(ctx context.Context, addr string, node string) (*Ingester, error) {
	var remote string

	c := &Ingester{
		name:    fmt.Sprintf("loki_%s", node),
		Labels:  fmt.Sprintf(`{instance = "%s",job = "inspector"}`, node),
		quit:    make(chan struct{}),
		entries: make(chan *logproto.Entry, LogEntriesChanSize),
		client:  httpClient{},
	}

	if net.ParseIP(addr) != nil {
		// use ip directly
		remote = fmt.Sprintf("http://%s:3100/api/prom/push", addr)
	} else {
		// try to resolve the addr as a domain name
		svr, err := net.LookupIP(addr)
		if err != nil {
			slog.Ctx(ctx).Warn("new loki ingester", "err", err, "addr", addr)
			return c, fmt.Errorf("invalid loki remote address %s", addr)
		}
		if len(svr) == 0 {
			slog.Ctx(ctx).Warn("new loki ingester", "err", "no such host", "addr", addr)
			return c, fmt.Errorf("invalid loki remote address %s", addr)
		}
		remote = fmt.Sprintf("http://%s:3100/api/prom/push", svr[0].String())
	}
	c.PushURL = remote
	c.waitGroup.Add(1)
	go c.startLoop(ctx)

	return c, nil
}

func (i *Ingester) Watch(ctx context.Context, datach chan inspproto.RawEvent) {
	for evt := range datach {
		et, err := nettop.GetEntityByNetns(int(evt.Netns))
		if err != nil {
			slog.Ctx(ctx).Warn("watch get entity", "err", err, "netns", evt.Netns, "client", i.Name())
			continue
		}

		evtStr := fmt.Sprintf(eventTmpl, evt.EventType, et.GetPodName(), et.GetPodNamespace(), evt.EventBody)
		now := time.Now().UnixNano()
		i.entries <- &logproto.Entry{
			Timestamp: &timestamp.Timestamp{
				Seconds: now / int64(time.Second),
				Nanos:   int32(now % int64(time.Second)),
			},
			Line: evtStr,
		}
	}
}

func (i *Ingester) Name() string {
	return i.name
}

type Ingester struct {
	name      string
	PushURL   string
	Labels    string
	quit      chan struct{}
	entries   chan *logproto.Entry
	waitGroup sync.WaitGroup
	client    httpClient
}

func (i *Ingester) startLoop(ctx context.Context) {
	var batch []*logproto.Entry
	batchSize := 0
	maxWait := time.NewTimer(5 * time.Second)

	defer func() {
		if batchSize > 0 {
			i.send(ctx, batch)
		}
		i.waitGroup.Done()
	}()

	for {
		select {
		case <-i.quit:
			return
		case entry := <-i.entries:
			batch = append(batch, entry)
			batchSize++
			if batchSize >= BatchEntriesNumber {
				slog.Ctx(ctx).Debug("loki ingester send of size")
				i.send(ctx, batch)
				batch = []*logproto.Entry{}
				batchSize = 0
				maxWait.Reset(BatchWait)
			}
		case <-maxWait.C:
			if batchSize > 0 {
				i.send(ctx, batch)
				batch = []*logproto.Entry{}
				batchSize = 0
			}
			slog.Ctx(ctx).Debug("loki ingester send of maxwait")
			maxWait.Reset(BatchWait)
		}
	}
}

func (i *Ingester) send(ctx context.Context, entries []*logproto.Entry) {
	var streams []*logproto.Stream
	streams = append(streams, &logproto.Stream{
		Labels:  i.Labels,
		Entries: entries,
	})

	req := logproto.PushRequest{
		Streams: streams,
	}

	buf, err := proto.Marshal(&req)
	if err != nil {
		slog.Ctx(ctx).Warn("loki ingester marshal request", "err", err)
		return
	}

	buf = snappy.Encode(nil, buf)

	resp, body, err := i.client.sendJSONReq("POST", i.PushURL, "application/x-protobuf", buf)
	if err != nil {
		slog.Ctx(ctx).Warn("loki ingester request error", "err", err)
		return
	}

	if resp.StatusCode != 204 {
		slog.Ctx(ctx).Warn("loki ingester response error", "status", resp.StatusCode, "body", body)
		return
	}
}

func (client *httpClient) sendJSONReq(method, url string, ctype string, reqBody []byte) (resp *http.Response, resBody []byte, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", ctype)

	resp, err = client.parent.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	resBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return resp, resBody, nil
}
