# fnos-qb-proxy

## What is it?

fnOS 中自带了一个下载器（基于 qBittorrent 和 Aria2），但默认关闭了 WebUI，且采用动态密码。这使得我们无法在外部连接 fnOS 中的 qBittorrent（e.g. 接入 MoviePilot 或 NasTools 等）

该项目是一个简单的代理，能绕过这些限制，提供在外部访问 fnOS 的 qBittorrent 的能力同时不影响 fnOS 自身的下载器运行


## Get Started

### Manual Install

下载 binary 到 fnOS 节点上

```bash
$ wget https://github.com/xxxuuu/fnos-qb-proxy/releases/download/v0.1.0/fnos-qb-proxy_linux-amd64 -O fnos-qb-proxy
$ chmod +x fnos-qb-proxy
```

参数，使用 `--uds` 指定 qBittorrent 监听的 Unix domain socket. 一般在 `/home/{user}/qbt.sock` 上
```bash
$ fnos-qb-proxy -h
NAME:
   fnos-qb-proxy - fnos-qb-proxy is a proxy for qBittorrent in fnOS

USAGE:
   fnos-qb-proxy [global options] command [command options]

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --password value, -p value  if not set, any password will be accepted
   --uds value                 qBittorrent unix domain socket(uds) path (default: "/home/admin/qbt.sock")
   --debug, -d                 (default: false)
   --port value                proxy running port (default: 8080)
   --help, -h                  show help
```

运行后，访问 `http://{host}:8080` 即可进入 qBittorrent WebUI。默认情况下用户名为 `admin`，输入任意密码均可访问，如果通过 `--password` 指定了密码，则只有该密码可访问；`--port` 修改运行端口
```bash
$ ./fnos-qb-proxy --uds "/home/admin/qbt.sock"
proxy running on port 8080
```

### Configure Systemd Service
上面的命令会一直在前台运行，可以使用 Systemd 配置成 daemon 在后台自动运行

移动 binary 到 `/usr/bin`
```bash
$ sudo mv fnos-qb-proxy /usr/bin/
```

将以下配置写入到 `/etc/systemd/system/fnos-qb-proxy.service`，可自行修改命令参数
```
[Unit]
Description=fnOS qBittorrent Proxy Service
Before=dlcenter.service

[Service]
ExecStart=/usr/bin/fnos-qb-proxy --uds "/home/admin/qbt.sock"
Restart=always

[Install]
WantedBy=multi-user.target
```

启用服务
```bash
$ sudo systemctl daemon-reload
$ sudo systemctl enable --now fnos-qb-proxy
```

查看服务状态，成功运行
```bash
$ sudo systemctl status fnos-qb-proxy
● fnos-qb-proxy.service - fnOS qBittorrent Proxy Service
     Loaded: loaded (/etc/systemd/system/fnos-qb-proxy.service; enabled; preset: enabled)
     Active: active (running) since Mon 2024-10-21 23:09:34 CST; 4s ago
   Main PID: 1801543 (fnos-qb-proxy)
      Tasks: 6 (limit: 9495)
     Memory: 6.0M
        CPU: 122ms
     CGroup: /system.slice/fnos-qb-proxy.service
             └─1801543 /usr/bin/fnos-qb-proxy --uds /home/admin/qbt.sock

Oct 21 23:09:34 fnOS systemd[1]: Started fnos-qb-proxy.service - fnOS qBittorrent Proxy Service.
Oct 21 23:09:34 fnOS fnos-qb-proxy[1801543]: proxy running on port 8080
```



### Docker Install

See [Docker-Install.md](/Docker-Install.md). 
