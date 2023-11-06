package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/config"
	"github.com/kubeskoop/webconsole/internal/handler"
)

func main() {
	router := gin.Default()
	if gin.Mode() != gin.ReleaseMode {
		router.Use(cors.New(cors.Config{
			AllowAllOrigins: true,
			AllowMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:    []string{"Content-Type", "Authorization"},
		}))
	}

	auth, err := handler.GetAuthMiddleware()
	if err != nil {
		log.Fatal(err)
	}
	router.Use(static.ServeRoot("/", config.Global.StaticDir))

	g := router.Group("/grafana")
	handler.RegisterGrafanaProxyHandler(g, auth)

	apiGroup := router.Group("/api")
	g = apiGroup.Group("/auth")
	handler.RegisterAuthHandler(g, auth)
	g = apiGroup.Group("/config")
	handler.RegisterConfigHandler(g, auth)
	g = apiGroup.Group("/diagnosis")
	handler.RegisterDiagnosisHandler(g, auth)
	g = apiGroup.Group("/controller")
	handler.RegisterControllerHanler(g, auth)

	if err := router.Run(); err != nil {
		log.Fatal(err)
	}
}
