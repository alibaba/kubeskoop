package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	log "github.com/sirupsen/logrus"
	"strconv"
	"sync"
	"time"
)

type NodeInfo struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Nodename  string `json:"nodename"`
	Namespace string `json:"namespace"`
}

type Latency struct {
	Source     *NodeInfo `json:"source"`
	Target     *NodeInfo `json:"destination"`
	LatencyMax float64   `json:"latency_max"`
	LatencyMin float64   `json:"latency_min"`
	LatencyAvg float64   `json:"latency_avg"`
}

type PingMeshArgs struct {
	PingMeshList []NodeInfo `json:"ping_mesh_list"`
}

type PingMeshResult struct {
	Nodes []NodeInfo `json:"nodes"`
	//unit ms
	Latencies []Latency `json:"latencies"`
}

func (c *controller) dispatchPingTask(ctx context.Context, src, dst NodeInfo, taskGroup *sync.WaitGroup, latencyResult chan<- *Latency) error {
	taskId := strconv.Itoa(int(getTaskIdx()))
	pingInfo := &rpc.PingInfo{}
	var err error
	switch src.Type {
	case "Pod":
		pingInfo.Pod, pingInfo.Node, _, err = c.getPodInfo(ctx, src.Namespace, src.Name)
		if err != nil {
			return err
		}
	case "Node":
		src.Nodename = src.Name
		pingInfo.Node, _, err = c.getNodeInfo(ctx, src.Name)
		if err != nil {
			return err
		}
	}
	switch dst.Type {
	case "Pod":
		_, _, pingInfo.Destination, err = c.getPodInfo(ctx, dst.Namespace, dst.Name)
		if err != nil {
			return err
		}
	case "Node":
		_, pingInfo.Destination, err = c.getNodeInfo(ctx, dst.Name)
		if err != nil {
			return err
		}
	}

	_, err = c.commitTask(src.Nodename, &rpc.Task{
		Type: rpc.TaskType_Ping,
		Id:   taskId,
		TaskInfo: &rpc.Task_Ping{
			Ping: pingInfo,
		},
	})
	if err != nil {
		return err
	}
	taskGroup.Add(1)
	go func() {
		defer taskGroup.Done()
		result, err := c.waitTaskResult(ctx, taskId)
		if err != nil || result.Success == false {
			if err != nil {
				log.Errorf("wait task result error: %v", err)
			} else {
				log.Errorf("wait task result error result: %+v", result.Message)
			}
			latencyResult <- &Latency{
				Source:     &src,
				Target:     &dst,
				LatencyAvg: 9999.9,
				LatencyMax: 9999.9,
				LatencyMin: 9999.9,
			}
			return
		}
		if pingResult := result.GetPing(); pingResult != nil {
			latencyResult <- &Latency{
				Source:     &src,
				Target:     &dst,
				LatencyAvg: float64(pingResult.GetAvg()),
				LatencyMax: float64(pingResult.GetMax()),
				LatencyMin: float64(pingResult.GetMin()),
			}
		}
	}()
	return nil
}

func (c *controller) PingMesh(ctx context.Context, pingmesh *PingMeshArgs) (*PingMeshResult, error) {
	log.Infof("PingMesh: %+v", pingmesh)
	taskGroup := sync.WaitGroup{}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	latencyResult := make(chan *Latency, len(pingmesh.PingMeshList)*len(pingmesh.PingMeshList))
	pingResult := &PingMeshResult{}
	var err error
	for sidx, src := range pingmesh.PingMeshList {
		pingResult.Nodes = append(pingResult.Nodes, src)
		for didx, dst := range pingmesh.PingMeshList {
			if sidx == didx {
				continue
			}
			if err = c.dispatchPingTask(timeoutCtx, src, dst, &taskGroup, latencyResult); err != nil {
				log.Errorf("dispatch ping task error: %v", err)
			}
		}
	}
	taskGroup.Wait()
	close(latencyResult)
	for l := range latencyResult {
		pingResult.Latencies = append(pingResult.Latencies, *l)
	}
	if len(pingResult.Latencies) == 0 {
		return nil, fmt.Errorf("no ping latencies can be display: %v", err)
	}

	return pingResult, nil
}
