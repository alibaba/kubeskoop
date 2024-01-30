package cmd

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/graph"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/controller/service"
	exporter "github.com/alibaba/kubeskoop/pkg/exporter/cmd"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"
)

const (
	defaultAgentPort = 10263
	defaultHTTPPort  = 10264
)

type Server struct {
	controller service.ControllerService
}

func NewServer() *Server {
	ctrlSVC, err := service.NewControllerService()
	if err != nil {
		log.Fatalf("error create controller service: %v", err)
	}
	return &Server{
		controller: ctrlSVC,
	}
}

func (s *Server) Run(agentPort int, httpPort int) {
	done := make(chan struct{})
	go s.RunAgentServer(agentPort, done)
	go s.RunHTTPServer(httpPort, done)
	go s.controller.Run(done)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	close(done)
}

func (s *Server) RunAgentServer(port int, done <-chan struct{}) {
	if port == 0 {
		port = defaultAgentPort
	}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(102 * 1024 * 1024))
	rpc.RegisterControllerRegisterServiceServer(grpcServer, s.controller)
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		log.Fatalf("err listen on %d: %v", port, err)
	}
	go func() {
		err = grpcServer.Serve(listener)
		if err != nil {
			log.Fatalf("grpc serve err: %v", err)
		}
	}()
	<-done
	grpcServer.Stop()
}

func (s *Server) RunHTTPServer(port int, done <-chan struct{}) {
	if port == 0 {
		port = defaultHTTPPort
	}
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.POST("/diagnose", s.CommitDiagnoseTask)
	r.GET("/diagnoses", s.ListDiagnoseTasks)
	r.POST("/capture", s.CommitCaptureTask)
	r.GET("/captures", s.ListCaptureTasks)
	r.GET("/capture/:task_id/download", s.DownloadCaptureFile)
	r.POST("/pingmesh", s.PingMesh)
	r.GET("/pods", s.ListPods)
	r.GET("/nodes", s.ListNodes)
	r.GET("/namespaces", s.ListNamespaces)
	r.GET("/flow", s.GetFlowGraph)
	r.GET("/events", s.GetEvent)
	r.GET("/config", s.GetExporterConfig)
	r.PUT("/config", s.UpdateExporterConfig)

	go func() {
		err := r.Run(fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			log.Fatalf("error run http server: %v", err)
		}
	}()
	<-done
}

// CommitDiagnoseTask commit diagnose task
func (s *Server) CommitDiagnoseTask(ctx *gin.Context) {
	var task skoopContext.TaskConfig
	if err := ctx.ShouldBindJSON(&task); err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error get task config from request: %v", err)})
		return
	}
	taskID, err := s.controller.Diagnose(ctx, &task)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error commit diagnose task: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, map[string]string{"task_id": fmt.Sprintf("%d", taskID)})
}

// ListDiagnoseTasks list all diagnose task
func (s *Server) ListDiagnoseTasks(ctx *gin.Context) {
	tasks, err := s.controller.DiagnoseList(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error list diagnose task: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, tasks)
}

// CommitCaptureTask commit capture task
func (s *Server) CommitCaptureTask(ctx *gin.Context) {
	var captureTask service.CaptureArgs
	if err := ctx.ShouldBindJSON(&captureTask); err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error get task config from request: %v", err)})
		return
	}
	taskID, err := s.controller.Capture(ctx, &captureTask)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error commit capture task: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, map[string]string{"task_id": fmt.Sprintf("%d", taskID)})
}

// ListCaptureTasks list all capture task
func (s *Server) ListCaptureTasks(ctx *gin.Context) {
	tasks, err := s.controller.CaptureList(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error list capture task: %v", err)})
	}
	ctx.AsciiJSON(http.StatusOK, tasks)
}

// DownloadCaptureFile download capture file
func (s *Server) DownloadCaptureFile(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("task_id"))
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error get task id from request: %v", err)})
		return
	}

	name, fl, fd, err := s.controller.DownloadCaptureFile(ctx, id)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error download capture file: %v", err)})
		return
	}
	defer fd.Close()
	ctx.Header("Content-Disposition", "attachment; filename="+name)
	ctx.Header("Content-Type", "application/text/plain")
	ctx.Header("Accept-Length", fmt.Sprintf("%d", fl))
	_, err = io.Copy(ctx.Writer, fd)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error transmiss capture file: %v", err)})
		return
	}
	ctx.Status(http.StatusOK)
}

func (s *Server) ListPods(ctx *gin.Context) {
	pods, err := s.controller.PodList(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error list pods: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, pods)
}

func (s *Server) ListNodes(ctx *gin.Context) {
	nodes, err := s.controller.NodeList(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error list nodes: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, nodes)
}

func (s *Server) ListNamespaces(ctx *gin.Context) {
	namespaces, err := s.controller.NamespaceList(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error list namespaces: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, namespaces)
}

func (s *Server) PingMesh(ctx *gin.Context) {
	var pingmesh service.PingMeshArgs
	if err := ctx.ShouldBindJSON(&pingmesh); err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error get task config from request: %v", err)})
		return
	}
	result, err := s.controller.PingMesh(ctx, &pingmesh)
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error do pingmesh: %v", err)})
		return
	}
	ctx.JSON(200, result)
}

func (s *Server) GetFlowGraph(ctx *gin.Context) {
	var ts time.Time
	t := ctx.Query("time")
	if t != "" {
		ti, err := strconv.Atoi(t)
		if err != nil {
			ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot convert timestamp: %v", err)})
		}
		ts = time.Unix(int64(ti), 0)
	} else {
		ts = time.Now()
	}
	result, _, err := s.controller.QueryPrometheus(ctx, "kubeskoop_flow_bytes", ts)
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error query flow metrics: %v", err)})
		return
	}
	vector := result.(model.Vector)

	podInfo, nodeInfo, err := s.controller.GetPodNodeInfoFromMetrics(ctx, ts)
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error get pod info from metrics: %v", err)})
		return
	}

	g, err := graph.FromVector(vector, podInfo, nodeInfo)
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error convert flow metrics to graph: %v", err)})
		return
	}
	g.SetEdgeBytesFromVector(vector)

	result, _, err = s.controller.QueryPrometheus(ctx, "kubeskoop_flow_packets", ts)
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error query flow metrics: %v", err)})
		return
	}
	vector = result.(model.Vector)
	g.SetEdgePacketsFromVector(vector)

	jstr, err := g.ToJSON()
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error marshalling to json: %v", err)})
		return
	}

	ctx.Data(http.StatusOK, gin.MIMEJSON, jstr)
}

func (s *Server) GetEvent(ctx *gin.Context) {
	start := ctx.Query("start")
	end := ctx.Query("end")
	limit := ctx.Query("limit")
	nodes := ctx.Query("nodes")
	namespaces := ctx.Query("namespaces")
	pods := ctx.Query("pods")
	types := ctx.Query("types")

	var startTime, endTime time.Time
	var limitCnt int
	var err error

	if start != "" {
		s, err := strconv.Atoi(start)
		if err != nil {
			ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("start time format error: %v", err)})
			return
		}
		startTime = time.Unix(int64(s), 0)
	} else {
		startTime = time.Now().Add(-10 * time.Minute)
	}

	if end != "" {
		e, err := strconv.Atoi(end)
		if err != nil {
			ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("end time format error: %v", err)})
			return
		}
		endTime = time.Unix(int64(e), 0)
	} else {
		endTime = time.Now()
	}

	if limit != "" {
		l, err := strconv.Atoi(limit)
		if err != nil {
			ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("limit time format error: %v", err)})
			return
		}
		limitCnt = l
	} else {
		limitCnt = 100
	}

	filters := map[string][]string{}
	if nodes != "" {
		filters["instance"] = strings.Split(nodes, ",")
	}

	if namespaces != "" {
		filters["namespace"] = strings.Split(namespaces, ",")
	}

	if pods != "" {
		filters["pod"] = strings.Split(pods, ",")
	}

	if types != "" {
		filters["type"] = strings.Split(types, ",")
	}

	evts, err := s.controller.QueryRangeEvent(ctx, startTime, endTime, filters, limitCnt)
	if err != nil {
		ctx.AsciiJSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error query event: %v", err)})
		return
	}

	// sort events by timestamp in descending order
	sort.Slice(evts, func(i, j int) bool {
		return evts[i].Timestamp > evts[j].Timestamp
	})

	ctx.AsciiJSON(http.StatusOK, evts)
}

func (s *Server) GetExporterConfig(ctx *gin.Context) {
	cfg, err := s.controller.GetExporterConfig(ctx)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error get exporter config: %v", err)})
		return
	}
	ctx.AsciiJSON(http.StatusOK, cfg)
}

func (s *Server) UpdateExporterConfig(ctx *gin.Context) {
	var cfg *exporter.InspServerConfig
	err := ctx.BindJSON(&cfg)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error unmarshal config: %v", err)})
		return
	}

	err = s.controller.UpdateExporterConfig(ctx, cfg)
	if err != nil {
		ctx.AsciiJSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("error update exporter config: %v", err)})
		return
	}
	ctx.Status(http.StatusOK)
}
