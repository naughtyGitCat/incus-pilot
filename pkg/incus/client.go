package incus

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const DefaultSocketPath = "/run/incus/unix.socket"

type Client struct {
	SocketPath string
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	return &Client{
		SocketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.DialTimeout("unix", socketPath, 10*time.Second)
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// ProxyHandler 透明代理 /api/incus/* -> Incus /1.0/*
func (c *Client) ProxyHandler() gin.HandlerFunc {
	// target 设置为根 host "http://localhost"，由 Director 精确拼装 /1.0 路径
	target, _ := url.Parse("http://localhost")

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = c.httpClient.Transport

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// 替换前缀 /api/incus -> /1.0
		req.URL.Path = strings.Replace(req.URL.Path, "/api/incus", "/1.0", 1)
		if req.URL.RawPath != "" {
			req.URL.RawPath = strings.Replace(req.URL.RawPath, "/api/incus", "/1.0", 1)
		}
		req.Host = "localhost"
	}

	return func(ctx *gin.Context) {
		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}
