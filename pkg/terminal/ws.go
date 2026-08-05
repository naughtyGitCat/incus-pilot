package terminal

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// wsWrapper 包装 gorilla/websocket 成为 io.Reader 和 io.Writer
type wsWrapper struct {
	conn *websocket.Conn
	mu   sync.Mutex
	buf  []byte
}

func (w *wsWrapper) Read(p []byte) (int, error) {
	if len(w.buf) > 0 {
		n := copy(p, w.buf)
		w.buf = w.buf[n:]
		return n, nil
	}

	_, msg, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	n := copy(p, msg)
	if n < len(msg) {
		w.buf = msg[n:]
	}
	return n, nil
}

func (w *wsWrapper) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// TerminalHandler 使用 Incus 官方 Go SDK 处理 PTY 终端交互
func TerminalHandler(socketPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		instanceName := c.Param("name")
		if instanceName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Instance name required"})
			return
		}

		cols, _ := strconv.Atoi(c.DefaultQuery("cols", "80"))
		rows, _ := strconv.Atoi(c.DefaultQuery("rows", "24"))
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}

		// 1. 连接 Incus 官方 Client
		client, err := incus.ConnectIncusUnix(socketPath, &incus.ConnectionArgs{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Incus daemon: " + err.Error()})
			return
		}

		// 2. 升级为 WebSocket
		wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		wrapper := &wsWrapper{conn: wsConn}

		// 3. 构建官方 Exec 请求
		req := api.InstanceExecPost{
			Command:     []string{"/bin/bash", "-l"},
			Environment: map[string]string{"TERM": "xterm-256color", "HOME": "/root"},
			WaitForWS:   true,
			Interactive: true,
			Width:       cols,
			Height:      rows,
		}

		execArgs := &incus.InstanceExecArgs{
			Stdin:  wrapper,
			Stdout: wrapper,
			Stderr: wrapper,
		}

		// 尝试执行 bash，若不存在备选 sh
		op, err := client.ExecInstance(instanceName, req, execArgs)
		if err != nil {
			req.Command = []string{"/bin/sh", "-l"}
			op, err = client.ExecInstance(instanceName, req, execArgs)
			if err != nil {
				wsConn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mFailed to exec in instance: "+err.Error()+"\x1b[0m\r\n"))
				return
			}
		}

		// 4. 等待操作结束
		_ = op.WaitContext(context.Background())
	}
}
