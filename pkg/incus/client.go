package incus

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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
	target, _ := url.Parse("http://localhost")

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = c.httpClient.Transport

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
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

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// EventsHandler 处理 /api/events WebSocket 订阅转发
func (c *Client) EventsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		wsConn, err := wsUpgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		dialer := websocket.Dialer{
			NetDialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", c.Value("socket").(string))
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		ctxWithValue := context.WithValue(context.Background(), "socket", c.SocketPath)
		incusWS, _, err := dialer.DialContext(ctxWithValue, "ws://localhost/1.0/events?type=operation", nil)
		if err != nil {
			wsConn.WriteMessage(websocket.TextMessage, []byte(`{"error": "Failed to subscribe to Incus events"}`))
			return
		}
		defer incusWS.Close()

		// 双向管道转发
		errChan := make(chan error, 2)
		go func() {
			for {
				msgType, msg, err := incusWS.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				if err := wsConn.WriteMessage(msgType, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		go func() {
			for {
				msgType, msg, err := wsConn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				if err := incusWS.WriteMessage(msgType, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		<-errChan
	}
}
