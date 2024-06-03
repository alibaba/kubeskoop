package handler

import (
	"net/http"
	"strings"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/service/config"
)

func RegisterConfigHandler(g *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	g.Use(auth.MiddlewareFunc())
	g.GET("/dashboard", getDashboardConfig)
	g.PUT("/dashboard", setDashboardConfig)
}

type dashboardConfig struct {
	PodDashboardURL  string `json:"pod_dashboard_url"`
	NodeDashboardURL string `json:"node_dashboard_url"`
}

func getDashboardConfig(ctx *gin.Context) {
	cfg, err := config.Service.GetDashboardConfig()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(200, dashboardConfig{
		PodDashboardURL:  cfg.PodDashboardURL,
		NodeDashboardURL: cfg.NodeDashboardURL,
	})
}

func setDashboardConfig(ctx *gin.Context) {
	var c dashboardConfig
	err := ctx.ShouldBindJSON(&c)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := config.DashboardConfig{
		PodDashboardURL:  c.PodDashboardURL,
		NodeDashboardURL: c.NodeDashboardURL,
	}
	if err := config.Service.SetDashboardConfig(cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": "ok"})
}
