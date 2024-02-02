package taskagent

import (
	"context"
	"os"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
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
	var conn *grpc.ClientConn
	var watchClient rpc.ControllerRegisterService_WatchTasksClient

	reconn := func() error {
		time.Sleep(1 * time.Second)
		log.Infof("connecting to controller.")
		if conn != nil {
			_ = conn.Close()
		}
		var err error
		conn, err = grpc.Dial(controllerAddr, grpc.WithDefaultCallOptions(opts...),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Errorf("failed to connect: %v", err)
			return err
		}
		a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)
		watchClient, err = a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
			NodeName: a.NodeName,
			Type:     []rpc.TaskType{rpc.TaskType_Capture, rpc.TaskType_Ping},
		})
		if err != nil {
			log.Errorf("failed to watch: %v", err)
			return err
		}
		log.Infof("controller connected.")
		return nil
	}

	err := reconn()
	if err != nil {
		return err
	}

	go func() {
		for {
			if watchClient == nil {
				_ = reconn()
				continue
			}
			select {
			case <-watchClient.Context().Done():
				log.Errorf("watch client closed")
				_ = reconn()
				continue
			default:
				task, err := watchClient.Recv()
				if err != nil {
					log.Errorf("failed to receive task: %v", err)
					_ = reconn()
					continue
				}
				err = a.ProcessTasks(task)
				if err != nil {
					log.Errorf("failed to process task: %v", err)
					continue
				}
			}
		}
	}()
	return nil
}

func (a *Agent) ProcessTasks(task *rpc.ServerTask) error {
	log.Infof("process task: %v", task)
	switch task.GetTask().GetType() {
	case rpc.TaskType_Capture:
		go func() {
			err := a.ProcessCapture(task)
			if err != nil {
				log.Errorf("failed to process capture: %v", err)
			}
		}()
		return nil
	case rpc.TaskType_Ping:
		go func() {
			err := a.ProcessPing(task)
			if err != nil {
				log.Errorf("failed to process ping: %v", err)
			}
		}()
		return nil
	}
	return nil
}
