package task_agent

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"os"
	"time"
)

var (
	//fixme replace to endpoint discovery
	controllerAddr = os.Getenv("CONTROLLER_SERVICE_HOST") + ":10263"
	//controllerAddr = "127.0.0.1:10263"
)

func NewTaskAgent() *Agent {
	return &Agent{NodeName: nettop.GetNodeName()}
}

type Agent struct {
	NodeName   string
	grpcClient rpc.ControllerRegisterServiceClient
}

func (a *Agent) Run() error {
	var opts []grpc.CallOption
	opts = append(opts, grpc.MaxCallSendMsgSize(102*1024*1024))
	conn, err := grpc.Dial(controllerAddr, grpc.WithDefaultCallOptions(opts...), grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)
	watchClient, err := a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
		NodeName: a.NodeName,
		Type:     []rpc.TaskType{rpc.TaskType_Capture, rpc.TaskType_Ping},
	})
	if err != nil {
		return fmt.Errorf("failed to watch tasks: %v", err)
	}
	reconn := func() {
		time.Sleep(1 * time.Second)
		conn, err = grpc.Dial(controllerAddr, grpc.WithDefaultCallOptions(opts...), grpc.WithInsecure())
		if err != nil {
			log.Errorf("failed to connect: %v", err)
			return
		}
		a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)
		watchClient, err = a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
			NodeName: a.NodeName,
			Type:     []rpc.TaskType{rpc.TaskType_Capture, rpc.TaskType_Ping},
		})
		if err != nil {
			log.Errorf("failed to watch: %v", err)
		}
	}

	go func() {
		defer conn.Close()
		for {
			select {
			case <-watchClient.Context().Done():
				log.Errorf("watch client closed")
				reconn()
				continue
			default:
				task, err := watchClient.Recv()
				if err != nil {
					log.Errorf("failed to receive task: %v", err)
					reconn()
					continue
				}
				a.ProcessTasks(task)
			}
		}
	}()
	return nil
}

func (a *Agent) ProcessTasks(task *rpc.ServerTask) error {
	log.Infof("process task: %v", task)
	switch task.GetTask().GetType() {
	case rpc.TaskType_Capture:
		go a.ProcessCapture(task)
		return nil
	case rpc.TaskType_Ping:
		go a.ProcessPing(task)
		return nil
	}
	return nil
}
