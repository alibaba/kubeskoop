package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/exporter/loki"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	jsoniter "github.com/json-iterator/go"
	"strings"
	"time"
)

type Event struct {
	Node string `json:"node"`
	Time int    `json:"time"`
	probe.Event
}

func buildFilterQueries(filter map[string][]string) string {
	var query []string
	for k, v := range filter {
		var cond []string
		for _, vv := range v {
			cond = append(cond, fmt.Sprintf("%s=\"%s\"", k, vv))
		}
		query = append(query, fmt.Sprintf("(%s)", strings.Join(cond, " or ")))
	}
	return strings.Join(query, " | ")
}

func (c *controller) QueryRangeEvent(ctx context.Context, start, end time.Time, labelFilter map[string][]string, limit int) ([]Event, error) {
	if c.lokiClient == nil {
		return nil, fmt.Errorf("loki client is not initialized")
	}
	query := "{job=\"kubeskoop\"} | json"
	if len(labelFilter) != 0 {
		filter := buildFilterQueries(labelFilter)
		query = fmt.Sprintf("%s | %s", query, filter)
	}
	resp, err := c.lokiClient.QueryRange(ctx, query, limit, start, end)
	if err != nil {
		return nil, err
	}
	if resp.Data.ResultType != lokiwrapper.ResultTypeStream {
		return nil, fmt.Errorf("expect result type \"streams\", but got %q", resp.Data.ResultType)
	}

	var ret []Event
	for _, r := range resp.Data.Result {
		node := r.Stream["instance"]
		for _, v := range r.Values {
			s := lokiwrapper.QueryResponseStream(v)
			ev := Event{
				Node: node,
				Time: s.NanoSecond(),
			}
			err := jsoniter.UnmarshalFromString(s.Log(), &ev.Event)
			if err != nil {
				return ret, err
			}
			ret = append(ret, ev)
		}
	}

	return ret, nil
}
