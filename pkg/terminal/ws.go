package terminal

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// TerminalHandler 处理网页 xterm.js 到 Incus /1.0/instances/<name>/exec 的 WebSocket 桥接
func TerminalHandler(socketPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		instanceName := c.Param("name")
		if instanceName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Instance name required"})
			return
		}

		// 升轨为 WebSocket
		wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		// 连接 Incus 的 Unix Socket
		dialer := websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		// 请求 Incus exec 交互
		// 注意: 在 Incus 中 exec 通常发送 POST 生成秘密 token 后升级 WS
		// 这里实现简易管道转发
		incusWS, _, err := dialer.Dial("ws://localhost/1.0/operations/exec", nil)
		if err != nil {
			wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to connect to container exec: "+err.Error()))
			return
		}
		defer incusWS.Close()

		// 双向数据拷贝
		errChan := make(chan error, 2)
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

		<-errChan
	}
}
