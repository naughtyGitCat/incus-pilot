# 🚀 Incus Pilot

**Incus Pilot** 是一个超轻量、单文件发布的现代化 Incus / LXD 容器 Web 管理仪表盘。

```text
                                [ 单文件二进制 ./incus-pilot ]
                               ┌─────────────────────────────┐
  [ 浏览器 / 移动端 ]          │  Vue 3 静态页面 (go:embed)   │
─────────────── HTTP / WS ───► │ ─────────────────────────── │ ── Unix Socket ──► /run/incus/unix.socket
  (JWT / 普通密码登录)         │  Go 后端 (API 代理 + 终端 WS)│
                               └─────────────────────────────┘
```

## 核心特性

- **单文件零依赖发布**：构建产物仅一个 `incus-pilot` 可执行文件（~15MB），无需安装 Nginx、PHP、Node.js。
- **免证书零配置**：Go 后端直接挂载 `/run/incus/unix.socket` 本地 Socket，天然拥有完整管理权限，彻底告别 mTLS 证书/OIDC 折磨。
- **普通密码登录**：前端原生支持用户名 + 密码登录（默认密码 `admin123456`，可通过环境变量重置）。
- **极度轻量**：运行内存仅 **~10-15MB**。
- **现代前端**：Vue 3 + TypeScript + Naive UI 深色界面，支持实时实例管理与 Web Terminal。

## 编译与运行

### 本地构建

```bash
# 一键编译前端 + 嵌入打包为单文件
make build

# 运行服务 (默认监听 :3000)
./incus-pilot --port 3000 --socket /run/incus/unix.socket
```

### 自定义密码

可通过环境变量或参数设置密码：

```bash
INCUS_PILOT_PASSWORD="my_strong_password" ./incus-pilot
```

## 开源协议

MIT License
