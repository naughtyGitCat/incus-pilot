package incus

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
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

// ProxyHandler 将 /api/incus/* 路径透明代理到 Incus 的 Unix Socket /1.0/*
func (c *Client) ProxyHandler() gin.HandlerFunc {
	target, _ := url.Parse("http://localhost/1.0")

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = c.httpClient.Transport

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// 从 /api/incus/instances 剥离前缀，转为 /1.0/instances
		path := req.URL.Path
		if len(path) >= 10 && path[:10] == "/api/incus" {
			req.URL.Path = "/1.0" + path[10:]
		}
		req.Host = "localhost"
	}

	return func(ctx *gin.Context) {
		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}
