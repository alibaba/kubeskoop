package service

import (
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func (c *controller) QueryPrometheus(ctx context.Context, query string, ts time.Time) (model.Value, promv1.Warnings, error) {
	if c.promClient == nil {
		return nil, nil, errors.New("prometheus client is not initialized")
	}
	log.Infof("Querying prometheus %s", query)
	a := promv1.NewAPI(c.promClient)

	return a.Query(ctx, query, ts)
}

func (c *controller) GetPodNodeInfoFromMetrics(ctx context.Context, ts time.Time) (model.Vector, model.Vector, error) {
	if c.promClient == nil {
		return nil, nil, errors.New("prometheus client is not initialized")
	}
	a := promv1.NewAPI(c.promClient)

	podResult, _, err := a.Query(ctx, "kubeskoop_info_pod", ts)
	if err != nil {
		return nil, nil, err
	}
	podMatrix := podResult.(model.Vector)

	nodeResult, _, err := a.Query(ctx, "kubeskoop_info_node", ts)
	if err != nil {
		return nil, nil, err
	}
	nodeMatrix := nodeResult.(model.Vector)

	return podMatrix, nodeMatrix, nil
}
