package main

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/handler"
	"log"
)

func main() {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Content-Type", "Authorization"},
	}))

	group := router.Group("/config")
	handler.RegisterConfigHandler(group)
	group = router.Group("/grafana")
	handler.RegisterGrafanaProxyHandler(group)
	group = router.Group("/diagnosis")
	handler.RegisterDiagnosisHandler(group)

	if err := router.Run(); err != nil {
		log.Fatal(err)
	}
}
