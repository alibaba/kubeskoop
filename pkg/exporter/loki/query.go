package lokiwrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	queryPath      = "/loki/api/v1/query"
	queryRangePath = "/loki/api/v1/query_range"
	tailPath       = "/loki/api/v1/tail"
)

type Client struct {
	endpoint string
}

type ResultType string

const (
	ResultTypeStream = "streams"
)

type QueryResponseStream []interface{}

func (s QueryResponseStream) NanoSecond() int {
	u, _ := strconv.Atoi(s[0].(string))
	return u
}

func (s QueryResponseStream) Log() string {
	return s[1].(string)
}

type QueryResponseMetric []interface{}

func (m QueryResponseMetric) Unix() int {
	return m[0].(int)
}

func (m QueryResponseMetric) Value() string {
	return m[1].(string)
}

type QueryResponseData struct {
	ResultType string `json:"resultType"`
	Result     []struct {
		Metric map[string]string `json:"metric"`
		Stream map[string]string `json:"stream"`
		Values [][]interface{}   `json:"values"`
	} `json:"result"`
}

type QueryResponse struct {
	Status string            `json:"status"`
	Data   QueryResponseData `json:"data"`
}

func NewClient(endpoint string) (*Client, error) {
	return &Client{
		endpoint: endpoint,
	}, nil
}

func (i *Client) Query(ctx context.Context, query string, limit int, time time.Time) (*QueryResponse, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("limit", fmt.Sprintf("%d", limit))
	values.Set("time", fmt.Sprintf("%d", time.UnixNano()))

	return i.doQuery(ctx, queryPath, values)
}

func (i *Client) QueryRange(ctx context.Context, query string, limit int, start, end time.Time) (*QueryResponse, error) {
	values := url.Values{}
	values.Set("query", query)
	if limit != 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	values.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	values.Set("end", fmt.Sprintf("%d", end.UnixNano()))

	return i.doQuery(ctx, queryRangePath, values)
}

func (i *Client) doQuery(ctx context.Context, path string, values url.Values) (*QueryResponse, error) {
	u, err := url.Parse(i.endpoint)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath(path)
	u.RawQuery = values.Encode()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var ret *QueryResponse
	err = json.NewDecoder(resp.Body).Decode(&ret)
	return ret, err
}
