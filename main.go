package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/naughtyGitCat/incus-pilot/pkg/auth"
	"github.com/naughtyGitCat/incus-pilot/pkg/incus"
	"github.com/naughtyGitCat/incus-pilot/pkg/terminal"
)

//go:embed web/dist/*
var webFS embed.FS

func main() {
	port := flag.Int("port", 3000, "Port to listen on")
	socketPath := flag.String("socket", "/run/incus/unix.socket", "Path to Incus unix socket")
	password := flag.String("password", "admin123456", "Admin password for login")
	flag.Parse()

	if envPass := os.Getenv("INCUS_PILOT_PASSWORD"); envPass != "" {
		*password = envPass
	}

	auth.InitAuth(*password)
	incusClient := incus.NewClient(*socketPath)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 1. 公开接口
	r.POST("/api/login", auth.LoginHandler)

	// 2. 需要 JWT 认证的受保护 API
	api := r.Group("/api", auth.Middleware())
	{
		api.Any("/incus/*path", incusClient.ProxyHandler())
		api.GET("/ws/exec/:name", terminal.TerminalHandler(*socketPath))
		api.GET("/events", incusClient.EventsHandler())
	}

	// 3. 静态前端 Vue 3 (通过 go:embed 打包)
	distFS, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend: %v", err)
	}

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/favicon.svg" || path == "/favicon.ico" {
			c.FileFromFS("favicon.svg", http.FS(distFS))
			return
		}
		c.FileFromFS(path, http.FS(distFS))
	})

	fmt.Printf("🚀 Incus Pilot listening on http://0.0.0.0:%d\n", *port)
	fmt.Printf("🔗 Connected to Incus socket: %s\n", *socketPath)

	if err := r.Run(fmt.Sprintf(":%d", *port)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
