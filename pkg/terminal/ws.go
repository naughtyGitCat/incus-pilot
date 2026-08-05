package terminal

import (
	"bytes"
	"context"
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

type windowResizeArgs struct {
	Width  string `json:"width"`
	Height string `json:"height"`
}

type windowResizeMsg struct {
	Command string           `json:"command"`
	Args    windowResizeArgs `json:"args"`
}

// TerminalHandler 处理网页 xterm.js 到 Incus 的 PTY 交互
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

		// 2. 发起 POST /1.0/instances/<name>/exec
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
		secretZero := execRes.Metadata.Metadata.Fds.Zero
		secretControl := execRes.Metadata.Metadata.Fds.Control

		if secretZero == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No fd secret returned by Incus"})
			return
		}

		// 3. 升级为前端 WebSocket
		wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		dialer := websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}

		// 4.1 连接 Incus control 端口保持心跳并发 resize
		if secretControl != "" {
			controlURL := fmt.Sprintf("ws://localhost/1.0/operations/%s/websocket?secret=%s", opID, secretControl)
			controlWS, _, err := dialer.Dial(controlURL, nil)
			if err == nil {
				defer controlWS.Close()
				resizeData, _ := json.Marshal(windowResizeMsg{
					Command: "window-resize",
					Args: windowResizeArgs{
						Width:  "80",
						Height: "24",
					},
				})
				controlWS.WriteMessage(websocket.TextMessage, resizeData)

				go func() {
					for {
						if _, _, err := controlWS.ReadMessage(); err != nil {
							return
						}
					}
				}()
			}
		}

		// 4.2 连接 Incus 数据端口 fds.0
		dataURL := fmt.Sprintf("ws://localhost/1.0/operations/%s/websocket?secret=%s", opID, secretZero)
		incusWS, _, err := dialer.Dial(dataURL, nil)
		if err != nil {
			wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to dial Incus exec WS: "+err.Error()))
			return
		}
		defer incusWS.Close()

		// 5. 消息透传：将 IncusWS 接收到的消息以 Text 形式解包写入前端 xterm.js
		errChan := make(chan error, 2)

		// 前端 -> IncusWS
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

		// IncusWS -> 前端
		go func() {
			for {
				_, msg, err := incusWS.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				// 转换为 TextMessage 保证 xterm.js 正常渲染字符
				if err := wsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		<-errChan
	}
}
