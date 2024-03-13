package taskagent

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	NodeName      string
	grpcClient    rpc.ControllerRegisterServiceClient
	ipCacheClient rpc.IPCacheServiceClient
}

func (a *Agent) rpcConnect() (*grpc.ClientConn, error) {
	var opts []grpc.CallOption
	opts = append(opts, grpc.MaxCallSendMsgSize(102*1024*1024))
	return grpc.Dial(controllerAddr, grpc.WithDefaultCallOptions(opts...),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func retry(msg string, maxAttempts int, work func() error) error {
	attempts := 0
	backoff := 1 // unit: second
	for {
		err := work()
		if err == nil {
			return nil
		}
		attempts++
		if maxAttempts > 0 && attempts >= maxAttempts {
			return fmt.Errorf("failed %s after %d attempts", msg, attempts)
		}

		backoff = backoff * 2
		if backoff > 10 {
			backoff = 10
		}
		log.Warningf("retry %s after %d seconds", msg, backoff)

		time.Sleep(time.Duration(backoff) * time.Second)
	}
}

func (a *Agent) watchTask() error {
	var conn *grpc.ClientConn
	var watchClient rpc.ControllerRegisterService_WatchTasksClient

	reconn := func(maxAttempts int) error {
		return retry("watching task", maxAttempts, func() error {
			log.Infof("connecting to controller.")
			if conn != nil {
				_ = conn.Close()
			}
			var err error
			conn, err = a.rpcConnect()
			if err != nil {
				return err
			}

			a.grpcClient = rpc.NewControllerRegisterServiceClient(conn)

			watchClient, err = a.grpcClient.WatchTasks(context.TODO(), &rpc.TaskFilter{
				NodeName: a.NodeName,
				Type:     []rpc.TaskType{rpc.TaskType_Capture, rpc.TaskType_Ping},
			})
			if err != nil {
				log.Errorf("failed to watch task: %v", err)
				return err
			}
			log.Infof("controller connected.")
			return nil
		})
	}

	err := reconn(3)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-watchClient.Context().Done():
				log.Errorf("watch client closed")
				_ = reconn(-1)
				continue
			default:
				task, err := watchClient.Recv()
				if err != nil {
					log.Errorf("failed to receive task: %v", err)
					_ = reconn(-1)
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

func (a *Agent) syncIPCache() error {
	var conn *grpc.ClientConn
	var watchClient rpc.IPCacheService_WatchCacheClient

	entry2IPInfo := func(e *rpc.CacheEntry) *nettop.IPInfo {
		info := &nettop.IPInfo{
			IP: e.IP,
		}
		switch v := e.Meta.(type) {
		case *rpc.CacheEntry_Node:
			info.Type = nettop.IPTypeNode
			info.NodeName = v.Node.Name
		case *rpc.CacheEntry_Pod:
			info.Type = nettop.IPTypePod
			info.PodNamespace = v.Pod.Namespace
			info.PodName = v.Pod.Name
		default:
			return nil
		}
		return info
	}

	reconn := func(maxAttempts int, relist bool) error {
		return retry("watching ipcache", maxAttempts, func() error {
			log.Infof("connecting to controller.")
			if conn != nil {
				_ = conn.Close()
			}
			var err error
			conn, err = a.rpcConnect()
			if err != nil {
				return err
			}

			a.ipCacheClient = rpc.NewIPCacheServiceClient(conn)
			period, revision := nettop.IPCacheRevision()

			if period == "" {
				relist = true
			}

			if relist {
				log.Warnf("list ipcache")
				listResp, err := a.ipCacheClient.ListCache(context.TODO(), &rpc.ListCacheRequest{})
				if err != nil {
					return err
				}

				period = listResp.Period
				revision = listResp.Revision

				log.Infof("current period:%s, revision: %d", period, revision)

				var ipInfoSlice []*nettop.IPInfo
				for _, e := range listResp.Entries {
					info := entry2IPInfo(e)
					if info == nil {
						continue
					}
					ipInfoSlice = append(ipInfoSlice, info)
				}

				nettop.UpdateIPCache(period, revision, ipInfoSlice)
			}

			rq := &rpc.WatchCacheRequest{
				Period:   period,
				Revision: revision,
			}

			watchClient, err = a.ipCacheClient.WatchCache(context.TODO(), rq)
			if err != nil {
				log.Errorf("failed to watch ipcache: %v", err)
				return err
			}
			return nil
		})
	}

	err := reconn(3, false)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-watchClient.Context().Done():
				log.Errorf("ipcache watch client closed")
				_ = reconn(-1, false)
				continue
			default:
				resp, err := watchClient.Recv()
				if err != nil {
					log.Errorf("ipcache failed to receive task: %v", err)
					s, ok := status.FromError(err)
					if ok && (s.Code() == codes.DataLoss || s.Code() == codes.InvalidArgument) {
						_ = reconn(-1, true)
					} else {
						_ = reconn(-1, false)
					}
					continue
				}
				info := entry2IPInfo(resp.Entry)
				log.Debugf("ip cache changed op=%d,info=[%s]", resp.Opcode, info)
				nettop.ApplyIPCacheChange(resp.Revision, resp.Opcode, info)
			}
		}
	}()

	return nil
}

func (a *Agent) Run() error {
	if err := a.watchTask(); err != nil {
		return err
	}
	if err := a.syncIPCache(); err != nil {
		return err
	}
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
