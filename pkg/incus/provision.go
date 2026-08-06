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

type execOpResponse struct {
	Operation string `json:"operation"`
}

type opStatusResponse struct {
	Metadata struct {
		Status string `json:"status"`
		Err    string `json:"err"`
	} `json:"metadata"`
}

// ProvisionSSH 向容器内下发并阻塞等待 OpenSSH Server 安装与 SSH 密钥写入
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
		Timeout: 60 * time.Second,
	}

	script := fmt.Sprintf(`
if command -v dnf >/dev/null 2>&1; then
  dnf install -y openssh-server openssh-clients
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache openssh-server openssh
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

	respBody, _ := io.ReadAll(resp.Body)
	var opRes execOpResponse
	json.Unmarshal(respBody, &opRes)

	if opRes.Operation != "" {
		// 阻塞轮询等待下载与安装 Task 完成
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			opResp, err := unixClient.Get(fmt.Sprintf("http://localhost%s", opRes.Operation))
			if err != nil {
				continue
			}
			opBody, _ := io.ReadAll(opResp.Body)
			opResp.Body.Close()

			var st opStatusResponse
			if err := json.Unmarshal(opBody, &st); err == nil {
				if st.Metadata.Status == "Success" {
					log.Printf("[Provisioner] SSH provisioning completed successfully for %s!", instanceName)
					return nil
				} else if st.Metadata.Status == "Failure" {
					log.Printf("[Provisioner] SSH provisioning failed for %s: %s", instanceName, st.Metadata.Err)
					return fmt.Errorf(st.Metadata.Err)
				}
			}
		}
	}

	return nil
}
