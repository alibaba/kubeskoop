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

type configParam struct {
	MetricsURL string `json:"metrics_url"`
	EventURL   string `json:"event_url"`
	FlowURL    string `json:"flow_url"`
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
	ctx.JSON(200, gin.H{
		"metrics_url": cfg.MetricsURL,
		"event_url":   cfg.EventURL,
		"flow_url":    cfg.FlowURL,
	})
}

func setDashboardConfig(ctx *gin.Context) {
	var c configParam
	err := ctx.ShouldBindJSON(&c)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := config.DashboardConfig{
		MetricsURL: c.MetricsURL,
	}
	if err := config.Service.SetDashboardConfig(cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": "ok"})
}
