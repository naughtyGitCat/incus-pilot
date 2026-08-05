package terminal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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

type execPostPayload struct {
	Command          []string          `json:"command"`
	Environment      map[string]string `json:"environment"`
	WaitForWebsocket bool              `json:"wait-for-websocket"`
	Interactive      bool              `json:"interactive"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
}

type execResponse struct {
	Type     string `json:"type"`
	Metadata struct {
		ID       string `json:"id"`
		Metadata struct {
			Fds struct {
				Zero    string `json:"0"`
				Control string `json:"control"`
			} `json:"fds"`
		} `json:"metadata"`
	} `json:"metadata"`
}

// TerminalHandler 处理网页 xterm.js 到 Incus 两阶段 Exec WebSocket 桥接
func TerminalHandler(socketPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		instanceName := c.Param("name")
		if instanceName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Instance name required"})
			return
		}

		// 1. HTTP 客户端连接 Unix Socket
		unixClient := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		}

		// 2. 发起 POST /1.0/instances/<name>/exec 申请交互会话
		payload := execPostPayload{
			Command:          []string{"/bin/sh", "-l"},
			Environment:      map[string]string{"TERM": "xterm-256color", "HOME": "/root"},
			WaitForWebsocket: true,
			Interactive:      true,
			Width:            80,
			Height:           24,
		}
		bodyBytes, _ := json.Marshal(payload)

		execURL := fmt.Sprintf("http://localhost/1.0/instances/%s/exec", instanceName)
		resp, err := unixClient.Post(execURL, "application/json", bytes.NewBuffer(bodyBytes))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to request exec: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		var execRes execResponse
		if err := json.Unmarshal(respBody, &execRes); err != nil || execRes.Metadata.ID == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid exec response from Incus"})
			return
		}

		opID := execRes.Metadata.ID
		secret := execRes.Metadata.Metadata.Fds.Zero
		if secret == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No fd secret returned by Incus"})
			return
		}

		// 3. 升级为前端 WebSocket
		wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		// 4. 连接 Incus 的 WebSocket operation 端口: ws://localhost/1.0/operations/<id>/websocket?secret=<secret>
		dialer := websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		incusWSURL := fmt.Sprintf("ws://localhost/1.0/operations/%s/websocket?secret=%s", opID, secret)
		incusWS, _, err := dialer.Dial(incusWSURL, nil)
		if err != nil {
			wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to dial Incus exec WS: "+err.Error()))
			return
		}
		defer incusWS.Close()

		// 5. 双向管道转发 (Incus 要求传输二进制数据包)
		errChan := make(chan error, 2)
		go func() {
			for {
				_, msg, err := wsConn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				// 转换为 BinaryMessage 传给 Incus
				if err := incusWS.WriteMessage(websocket.BinaryMessage, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		go func() {
			for {
				_, msg, err := incusWS.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				// 将 Incus 传回的数据写回前端 xterm.js
				if err := wsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		<-errChan
	}
}
