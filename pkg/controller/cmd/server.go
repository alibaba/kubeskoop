package cmd

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/controller/service"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

const (
	defaultAgentPort = 10263
	defaultHttpPort  = 10264
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
		port = defaultHttpPort
	}
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.POST("/diagnose", s.CommitDiagnoseTask)
	r.GET("/diagnoses", s.ListDiagnoseTasks)
	r.POST("/capture", s.CommitCaptureTask)
	r.GET("/captures", s.ListCaptureTasks)
	r.GET("/capture/:task_id/download", s.DownloadCaptureFile)
	r.GET("/pods", s.ListPods)
	r.GET("/nodes", s.ListNodes)

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
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error get task config from request: %v", err)})
		return
	}
	taskID, err := s.controller.Diagnose(context.TODO(), &task)
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error commit diagnose task: %v", err)})
		return
	}
	ctx.AsciiJSON(200, map[string]string{"task_id": fmt.Sprintf("%d", taskID)})
}

// ListDiagnoseTask list all diagnose task
func (s *Server) ListDiagnoseTasks(ctx *gin.Context) {
	tasks, err := s.controller.DiagnoseList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list diagnose task: %v", err)})
	}
	ctx.AsciiJSON(200, tasks)
}

// CommitCaptureTask commit capture task
func (s *Server) CommitCaptureTask(ctx *gin.Context) {
	var captureTask service.CaptureArgs
	if err := ctx.ShouldBindJSON(&captureTask); err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error get task config from request: %v", err)})
		return
	}
	taskID, err := s.controller.Capture(context.TODO(), &captureTask)
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error commit capture task: %v", err)})
		return
	}
	ctx.AsciiJSON(200, map[string]string{"task_id": fmt.Sprintf("%d", taskID)})
}

// ListCaptureTask list all capture task
func (s *Server) ListCaptureTasks(ctx *gin.Context) {
	tasks, err := s.controller.CaptureList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list capture task: %v", err)})
	}
	ctx.AsciiJSON(200, tasks)
}

// DownloadCaptureFile download capture file
func (s *Server) DownloadCaptureFile(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("task_id"))
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error get task id from request: %v", err)})
		return
	}

	name, fl, fd, err := s.controller.DownloadCaptureFile(context.TODO(), id)
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error download capture file: %v", err)})
		return
	}
	defer fd.Close()
	ctx.Header("Content-Disposition", "attachment; filename="+name)
	ctx.Header("Content-Type", "application/text/plain")
	ctx.Header("Accept-Length", fmt.Sprintf("%d", fl))
	_, err = io.Copy(ctx.Writer, fd)
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error transmiss capture file: %v", err)})
		return
	}
	ctx.Status(http.StatusOK)
}

func (s *Server) ListPods(ctx *gin.Context) {
	pods, err := s.controller.PodList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list pods: %v", err)})
		return
	}
	ctx.AsciiJSON(200, pods)
}

func (s *Server) ListNodes(ctx *gin.Context) {
	nodes, err := s.controller.NodeList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list nodes: %v", err)})
		return
	}
	ctx.AsciiJSON(200, nodes)
}
