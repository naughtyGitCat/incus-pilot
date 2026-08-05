package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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

// TerminalHandler 使用 Raw Net.Conn 双向透传连接网页 xterm.js 与 Incus PTY 管道
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
					return net.DialTimeout("unix", socketPath, 10*time.Second)
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
		secretZero := execRes.Metadata.Metadata.Fds.Zero
		secretControl := execRes.Metadata.Metadata.Fds.Control

		if secretZero == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No fd secret returned by Incus"})
			return
		}

		// 3. Hijack 当前前端的 HTTP 连接，拿到底层的 net.Conn 原生 TCP 连接
		hj, ok := c.Writer.(http.Hijacker)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Webserver doesn't support hijacking"})
			return
		}
		clientConn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()

		// 4.1 连接 Incus control Unix Socket
		if secretControl != "" {
			go func() {
				controlConn, err := net.Dial("unix", socketPath)
				if err != nil {
					return
				}
				defer controlConn.Close()

				// 发送 WebSocket Upgrade 请求给 Incus control 接口
				reqStr := fmt.Sprintf("GET /1.0/operations/%s/websocket?secret=%s HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", opID, secretControl)
				controlConn.Write([]byte(reqStr))

				// 读走握手 101 头
				buf := make([]byte, 1024)
				controlConn.Read(buf)

				// 保持 control 管道连接不关闭
				for {
					if _, err := controlConn.Read(buf); err != nil {
						return
					}
				}
			}()
		}

		// 4.2 连接 Incus 数据通道 fds.0 Unix Socket
		incusDataConn, err := net.Dial("unix", socketPath)
		if err != nil {
			return
		}
		defer incusDataConn.Close()

		// 5. 组装请求：将前端带有的 Upgrade WebSocket Headers 直接透传发给 Incus 数据端口
		reqStr := fmt.Sprintf("GET /1.0/operations/%s/websocket?secret=%s HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", opID, secretZero)
		incusDataConn.Write([]byte(reqStr))

		// 6. 物理层原生字节双向复制拷贝 (Raw IO Pipe Copy)
		errChan := make(chan error, 2)
		go func() {
			_, err := io.Copy(incusDataConn, clientConn)
			errChan <- err
		}()

		go func() {
			_, err := io.Copy(clientConn, incusDataConn)
			errChan <- err
		}()

		<-errChan
	}
}
