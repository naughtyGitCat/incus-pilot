package incus

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

type instanceCreateReq struct {
	Name   string            `json:"name"`
	Config map[string]string `json:"config"`
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
		// 拦截 POST /api/incus/instances 捕获 SSH 密钥
		if ctx.Request.Method == http.MethodPost && strings.HasSuffix(ctx.Request.URL.Path, "/instances") {
			bodyBytes, err := io.ReadAll(ctx.Request.Body)
			if err == nil {
				ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				var reqData instanceCreateReq
				if err := json.Unmarshal(bodyBytes, &reqData); err == nil && reqData.Name != "" {
					sshKey := reqData.Config["user.ssh_key"]
					if sshKey != "" {
						// 启动后台协程监控容器就绪并自动安装 sshd 与写入 Key
						go c.autoProvisionLoop(reqData.Name, sshKey)
					}
				}
			}
		}

		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}

func (c *Client) autoProvisionLoop(instanceName string, sshKey string) {
	log.Printf("[AutoProvision] Watching instance %s for SSH setup...", instanceName)
	// 轮询等待容器完成创建并启动
	unixClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", c.SocketPath, 5*time.Second)
			},
		},
		Timeout: 5 * time.Second,
	}

	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		resp, err := unixClient.Get(fmt.Sprintf("http://localhost/1.0/instances/%s", instanceName))
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var statusRes struct {
			Metadata struct {
				Status string `json:"status"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(body, &statusRes); err == nil {
			if statusRes.Metadata.Status == "Running" {
				log.Printf("[AutoProvision] Instance %s is Running! Provisioning SSH...", instanceName)
				time.Sleep(2 * time.Second)
				c.ProvisionSSH(instanceName, sshKey)
				return
			}
		}
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
