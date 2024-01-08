package lokiwrapper

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	inspproto "github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/alibaba/kubeskoop/pkg/exporter/loki/logproto"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/golang/snappy"
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

func NewLokiIngester(addr string, node string) (*Ingester, error) {
	var remote string

	c := &Ingester{
		name:    fmt.Sprintf("loki_%s", node),
		Labels:  fmt.Sprintf(`{instance = "%s",job = "kubeskoop"}`, node),
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
			log.Warn("new loki ingester", "err", err, "addr", addr)
			return c, fmt.Errorf("invalid loki remote address %s", addr)
		}
		if len(svr) == 0 {
			log.Warn("new loki ingester", "err", "no such host", "addr", addr)
			return c, fmt.Errorf("invalid loki remote address %s", addr)
		}
		remote = fmt.Sprintf("http://%s:3100/api/prom/push", svr[0].String())
	}
	c.PushURL = remote
	c.waitGroup.Add(1)
	go c.startLoop()

	return c, nil
}

func (i *Ingester) Watch(datach chan inspproto.RawEvent) {
	for evt := range datach {
		et, err := nettop.GetEntityByNetns(int(evt.Netns))
		if err != nil {
			log.Warn("watch get entity", "err", err, "netns", evt.Netns, "client", i.Name())
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

func (i *Ingester) Send(data string) {
	now := time.Now().UnixNano()
	i.entries <- &logproto.Entry{
		Timestamp: &timestamp.Timestamp{
			Seconds: now / int64(time.Second),
			Nanos:   int32(now % int64(time.Second)),
		},
		Line: data,
	}
}

func (i *Ingester) Name() string {
	return i.name
}

func (i *Ingester) Close() error {
	i.quit <- struct{}{}
	i.waitGroup.Wait()
	return nil
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

func (i *Ingester) startLoop() {
	var batch []*logproto.Entry
	batchSize := 0
	maxWait := time.NewTimer(5 * time.Second)

	defer func() {
		if batchSize > 0 {
			i.send(batch)
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
				log.Debug("loki ingester send of size")
				i.send(batch)
				batch = []*logproto.Entry{}
				batchSize = 0
				maxWait.Reset(BatchWait)
			}
		case <-maxWait.C:
			if batchSize > 0 {
				i.send(batch)
				batch = []*logproto.Entry{}
				batchSize = 0
			}
			log.Debug("loki ingester send of maxwait")
			maxWait.Reset(BatchWait)
		}
	}
}

func (i *Ingester) send(entries []*logproto.Entry) {
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
		log.Warn("loki ingester marshal request", "err", err)
		return
	}

	buf = snappy.Encode(nil, buf)

	resp, body, err := i.client.sendJSONReq("POST", i.PushURL, "application/x-protobuf", buf)
	if err != nil {
		log.Warn("loki ingester request error", "err", err)
		return
	}

	if resp.StatusCode != 204 {
		log.Warn("loki ingester response error", "status", resp.StatusCode, "body", body)
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
