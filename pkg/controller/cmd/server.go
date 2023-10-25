package cmd

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/controller/service"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
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
	return &Server{
		controller: service.NewControllerService(),
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
	grpcServer := grpc.NewServer()
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
	ctx.AsciiJSON(200, map[string]string{"taskID": fmt.Sprintf("%d", taskID)})
}

// ListDiagnoseTask list all diagnose task
func (s *Server) ListDiagnoseTasks(ctx *gin.Context) {
	tasks, err := s.controller.DiagnoseList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list diagnose task: %v", err)})
	}
	ctx.AsciiJSON(200, tasks)
}

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
	ctx.AsciiJSON(200, map[string]string{"taskID": fmt.Sprintf("%d", taskID)})
}

func (s *Server) ListCaptureTasks(ctx *gin.Context) {
	tasks, err := s.controller.CaptureList(context.TODO())
	if err != nil {
		ctx.AsciiJSON(400, map[string]string{"error": fmt.Sprintf("error list capture task: %v", err)})
	}
	ctx.AsciiJSON(200, tasks)
}
