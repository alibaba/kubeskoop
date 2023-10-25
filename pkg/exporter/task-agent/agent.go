package task_agent

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"os"
	"os/exec"
	"syscall"
	"time"
)

var (
	//fixme replace to endpoint discovery
	controllerAddr = os.Getenv("CONTROLLER_SERVICE_HOST") + ":10263"
	//controllerAddr = "127.0.0.1:10263"
)

func NewTaskAgent(nodename string) *Agent {
	return &Agent{NodeName: nodename}
}

type Agent struct {
	NodeName   string
	grpcClient rpc.ControllerRegisterServiceClient
}

func (a *Agent) Run() error {
	conn, err := grpc.Dial(controllerAddr, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)
	watchClient, err := a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
		NodeName: a.NodeName,
		Type:     rpc.TaskType_Capture,
	})
	if err != nil {
		return fmt.Errorf("failed to watch tasks: %v", err)
	}
	reconn := func() {
		time.Sleep(1 * time.Second)
		conn, err = grpc.Dial(controllerAddr, grpc.WithInsecure())
		if err != nil {
			log.Errorf("failed to connect: %v", err)
			return
		}
		a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)
		watchClient, err = a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
			NodeName: a.NodeName,
			Type:     rpc.TaskType_Capture,
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
	pcapFile := "/tmp/" + task.Task.Id + ".pcap"
	cmd := exec.Command("tcpdump", "-i", "any", "-w", pcapFile, "host", task.GetTask().GetCapture().GetPod().GetIpv4())
	var (
		output         []byte
		err            error
		captureContent []byte
	)
	go func() {
		output, err = cmd.CombinedOutput()
	}()
	time.Sleep(10 * time.Second)
	cmd.Process.Signal(syscall.SIGTERM)
	time.Sleep(time.Second)
	if err == nil {
		captureContent, err = os.ReadFile(pcapFile)
	}
	if err != nil {
		log.Errorf("failed to run command: %v, output: %s", err, output)
		a.grpcClient.UploadTaskResult(context.TODO(), &rpc.TaskResult{
			Id:      task.Task.Id,
			Type:    task.Task.Type,
			Success: false,
			Message: fmt.Sprintf("failed to run command: %v, output: %s", err, string(output)),
		})
		return err
	}

	a.grpcClient.UploadTaskResult(context.TODO(), &rpc.TaskResult{
		Id:      task.Task.Id,
		Type:    task.Task.Type,
		Success: true,
		Message: "success",
		TaskResultInfo: &rpc.TaskResult_Capture{Capture: &rpc.CaptureResult{
			Message: captureContent,
		}},
	})

	return nil
}
