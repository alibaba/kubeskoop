package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/config"
	"github.com/kubeskoop/webconsole/internal/service/controller"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func RegisterGrafanaProxyHandler(g *gin.RouterGroup) {
	g.Any("/*path", proxyHandler)
}

func RegisterControllerHanler(g *gin.RouterGroup) {
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
	path := ctx.Param("path")
	path = strings.TrimPrefix(path, "/")
	var (
		body []byte
		err  error
	)
	if ctx.Request.Body != nil {
		body, err = io.ReadAll(ctx.Request.Body)
		if err != nil {
			ctx.String(http.StatusBadRequest, "read body failed: %s", err.Error())
			return
		}
	}
	code, respFd, err := controller.Service.Proxy(path, ctx.Request.Method, body)
	if err != nil {
		ctx.String(http.StatusBadRequest, "proxy failed: %s", err.Error())
		return
	}
	defer respFd.Close()
	if _, err := io.Copy(ctx.Writer, respFd); err != nil {
		ctx.String(http.StatusBadRequest, "copy response failed: %s", err.Error())
		return
	}
	ctx.Status(code)
}
