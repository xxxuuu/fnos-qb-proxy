# fnos-qb-proxy

`fnos-qb-proxy` 是一个 HTTP 反向代理，用于访问 fnOS 内置的 qBittorrent 下载器。

fnOS 的下载器基于 qBittorrent 和 Aria2，qBittorrent 默认通过 Unix socket 提供 WebUI，例如：

```text
/home/kevin/qbt.sock
```

本项目把它转发为普通的 HTTP 服务，方便浏览器、MoviePilot、NasTools 等客户端访问：

```text
客户端 -> fnos-qb-proxy:8080 -> qBittorrent Unix socket
```

## 工作方式

fnOS 的 `dlcenter` 会在每次启动 qBittorrent 时生成随机 WebUI 密码。

当前版本的 fnOS qBittorrent 会把命令行中的密码改写成星号，例如：

```text
--webui-password=********
```

代理的密码获取顺序如下：

1. 首先从 qBittorrent 的命令行读取 `--webui-password`。
2. 如果密码已经被改写为星号，则读取 `dlcenter` 的可读进程内存。
3. 在内存中查找 dlcenter 登录 qBittorrent 时生成的请求内容：

   ```text
   username=admin&password=<真实密码>
   ```

4. 使用获得的密码登录 qBittorrent，并缓存 SID。

内存扫描范围受到限制，只扫描可读内存区域，匹配固定请求前缀，并对候选密码去重。qBittorrent 刚启动时如果 dlcenter 尚未完成登录，代理会短暂重试。


### 权限要求

读取其他进程的 `/proc/<pid>/mem` 通常需要 root 或等效的 ptrace 权限。因此新版 fnOS 的自动登录功能建议通过 root 身份运行：

- systemd 服务默认以 root 运行；
- 手动前台运行时使用 `sudo`；
- Docker 部署需要宿主机 PID namespace 和相应的 ptrace 权限，详见 [Docker-Install.md](Docker-Install.md)。

如果权限不足，代理会无法恢复被 qBittorrent 隐藏的密码，并在日志中报告错误。

## 手动安装

### 1. 下载并安装 binary

在 fnOS 节点上执行。写入 `/usr/bin` 通常需要 `sudo`：

```bash
wget https://github.com/stkevintan/fnos-qb-proxy/releases/latest/download/fnos-qb-proxy_linux-amd64 \
  -O /tmp/fnos-qb-proxy
chmod +x /tmp/fnos-qb-proxy
sudo install -o root -g root -m 0755 \
  /tmp/fnos-qb-proxy /usr/bin/fnos-qb-proxy
```

查看帮助：

```bash
sudo /usr/bin/fnos-qb-proxy --help
```

### 2. 直接前台运行

由于新版 fnOS 通常需要读取 dlcenter 内存，建议使用 `sudo`：

```bash
sudo /usr/bin/fnos-qb-proxy
```

默认监听 TCP `8080`。浏览器访问：

```text
http://<fnOS-IP>:8080
```

如果指定了 `--password`，访问代理时需要使用这个密码；如果不指定，代理会自动登录 fnOS qBittorrent：

```bash
sudo /usr/bin/fnos-qb-proxy --password 'your-proxy-password' --port 8080
```

停止前台进程时按 `Ctrl+C`。

## 配置 systemd

使用 systemd 可以让代理在后台运行，并确保它在下载中心之前启动。创建：

```bash
sudo tee /etc/systemd/system/fnos-qb-proxy.service >/dev/null <<'EOF'
[Unit]
Description=fnOS qBittorrent Proxy Service
Before=dlcenter.service
Wants=dlcenter.service

[Service]
Type=simple
ExecStart=/usr/bin/fnos-qb-proxy
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fnos-qb-proxy.service
```

检查状态和日志：

```bash
sudo systemctl status fnos-qb-proxy.service
sudo journalctl -u fnos-qb-proxy.service -f
```

服务启动后访问：

```text
http://<fnOS-IP>:8080
```

### 注意事项

- `Before=dlcenter.service` 只在两个服务由 systemd 同时启动时控制启动顺序。
- 如果 dlcenter 已经运行，建议重启一次下载中心，让代理能够观察到当前 qBittorrent 和 dlcenter 状态：

  ```bash
  sudo systemctl restart dlcenter.service
  ```

- 如果代理重启，密码会再次从 dlcenter 内存中读取，不需要重新安装或替换 qBittorrent。
- 如果 qBittorrent 登录失败返回 `403`，不要连续重试，否则可能触发 qBittorrent 的登录封禁。
- 不建议将 `8080` 直接暴露到公网。需要外网访问时，请配置代理密码、防火墙和访问控制。

## Docker

Docker 使用方式请参考 [Docker-Install.md](Docker-Install.md)。Docker 部署不修改宿主机的 qBittorrent binary；新版 fnOS 的内存读取需要额外的宿主机 PID namespace、root 权限和 ptrace 能力。

## 开发

运行测试：

```bash
go test ./...
```
