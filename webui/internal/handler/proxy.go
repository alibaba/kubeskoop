package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/config"
)

func RegisterGrafanaProxyHandler(g *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	g.Use(auth.MiddlewareFunc())
	g.Any("/*path", proxyHandler)
}

func RegisterControllerHanler(g *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	g.Use(auth.MiddlewareFunc())
	g.Any("/*path", proxyControllerHandler)
}

func proxyHandler(ctx *gin.Context) {
	host := config.Global.Grafana.Endpoint
	path := ctx.Param("path")
	remote, err := url.Parse(host + path)
	if err != nil {
		ctx.String(http.StatusBadRequest, "parse url failed: %s", err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(remote)
	proxy.Director = func(req *http.Request) {
		req.Header.Del("Origin")
		req.Host = remote.Host
		req.URL.Scheme = remote.Scheme
		req.URL.Host = remote.Host
		req.URL.Path = remote.Path
		if config.Global.Grafana.Username != "" && config.Global.Grafana.Password != "" {
			req.SetBasicAuth(config.Global.Grafana.Username, config.Global.Grafana.Password)
		}
	}
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
}

func proxyControllerHandler(ctx *gin.Context) {
	// todo whitelist for path
	host := config.Global.Controller.Endpoint
	path := ctx.Param("path")
	remote, err := url.Parse(host + path)
	if err != nil {
		ctx.String(http.StatusBadRequest, "parse url failed: %s", err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(remote)
	proxy.Director = func(req *http.Request) {
		req.Header.Del("Origin")
		req.Host = remote.Host
		req.URL.Scheme = remote.Scheme
		req.URL.Host = remote.Host
		req.URL.Path = remote.Path
	}
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
}
