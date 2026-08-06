package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type execSimplePayload struct {
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`
	WaitForWS   bool              `json:"wait-for-websocket"`
	Interactive bool              `json:"interactive"`
}

// ProvisionSSH 在容器启动后，直接通过 Incus 原生 Exec API 向容器内注入 OpenSSH Server 与公钥
func (c *Client) ProvisionSSH(instanceName string, sshKey string) error {
	cleanKey := strings.TrimSpace(sshKey)
	if cleanKey == "" {
		return nil
	}

	log.Printf("[Provisioner] Starting SSH provisioning for instance %s...", instanceName)

	unixClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", c.SocketPath, 10*time.Second)
			},
		},
		Timeout: 45 * time.Second,
	}

	// 构造多包管理器兼容的自动化安装与 SSH 服务启动脚本
	script := fmt.Sprintf(`
if command -v apk >/dev/null 2>&1; then
  apk add --no-cache openssh-server openssh
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y openssh-server openssh-clients
elif command -v apt-get >/dev/null 2>&1; then
  apt-get update && apt-get install -y openssh-server
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --noconfirm openssh
fi

mkdir -p /root/.ssh && chmod 700 /root/.ssh
echo '%s' > /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys

ssh-keygen -A 2>/dev/null || true
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config 2>/dev/null || true
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config 2>/dev/null || true

if command -v systemctl >/dev/null 2>&1; then
  systemctl enable --now sshd 2>/dev/null || systemctl enable --now ssh 2>/dev/null || true
elif command -v service >/dev/null 2>&1; then
  service sshd start 2>/dev/null || service ssh start 2>/dev/null || true
else
  /usr/sbin/sshd 2>/dev/null || /usr/sbin/sshd-keygen 2>/dev/null || true
fi
`, cleanKey)

	payload := execSimplePayload{
		Command:          []string{"/bin/sh", "-c", script},
		Environment:      map[string]string{"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		WaitForWS:   false,
		Interactive:      false,
	}

	bodyBytes, _ := json.Marshal(payload)
	execURL := fmt.Sprintf("http://localhost/1.0/instances/%s/exec", instanceName)

	resp, err := unixClient.Post(execURL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("[Provisioner] Exec request error for %s: %v", instanceName, err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[Provisioner] Exec response for %s: %s", instanceName, string(body))
	return nil
}
