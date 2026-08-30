# Docker 部署说明

## 镜像

镜像发布在 [ghcr.io/xxxuuu/fnos-qb-proxy](https://github.com/xxxuuu/fnos-qb-proxy/pkgs/container/fnos-qb-proxy)，可用标签示例：

| 标签 | 说明 |
| --------------------------------- | ------------------ |
| `latest` | 最新版本            |
| `v1.0.0` / `1.0.0` | 发版 tag           |
| commit 短 ID | 对应提交的开发版   |

## 部署要求

容器内的代理需要与宿主机上的 qBittorrent 进程交互，启动时有两处关键配置：

- **`--pid host`**：使用宿主机 PID namespace。代理通过 `/proc` 找到 fnOS 的 qBittorrent 进程，并从其命令行中解析 WebUI 密码与 UDS 路径。
- **只读挂载 UDS 所在目录**：qBittorrent 的 WebUI 通过 Unix socket（`qbt.sock`）提供，该 socket 必须在容器内以与宿主机相同的绝对路径可访问。其位置随 fnOS 版本有所不同：

  | fnOS 版本 | UDS 所在目录 |
  | --------- | ------------------------------ |
  | 旧版本 | `/home`                        |
  | 较新版本 | `/usr/trim/var/downloadcenter` |

  为兼容不同版本的 fnOS，两个目录都需要挂载。

## 方式 A：Docker Compose + WebUI（推荐）

在 fnOS WebUI 上直接运行容器时无法配置存储空间以外的挂载点，因此推荐使用 Compose 方式部署：

1. 在 fnOS WebUI → Docker → Compose 中新建项目，项目名称与路径可自定
2. 将项目中的 [docker-compose.yml](./docker-compose.yml) 内容上传或粘贴到 Compose 配置中
3. 按需修改密码与端口（见[配置说明](#配置说明)），可在本地编辑后上传，也可在 WebUI 中直接修改
4. 保存并启动

## 方式 B：`docker run`

需通过 SSH 连接到机器上执行命令（不熟悉命令行的建议使用方式 A）：

```bash
docker run -d \
  --name fnOS-qBit-Proxy \
  --pid host \
  -e PORT=8086 \
  -e PASSWORD=fnosnb \
  -p 7777:8086 \
  -v /home:/home:ro \
  -v /usr/trim/var/downloadcenter:/usr/trim/var/downloadcenter:ro \
  ghcr.io/xxxuuu/fnos-qb-proxy:latest
```

| 参数 | 说明 |
| --- | --- |
| `-d` | 后台运行容器 |
| `--name fnOS-qBit-Proxy` | 容器名称 |
| `--pid host` | 见[部署要求](#部署要求) |
| `-v /home:/home:ro` | 只读挂载旧版 fnOS 的 UDS 所在目录 |
| `-v /usr/trim/var/downloadcenter:/usr/trim/var/downloadcenter:ro` | 只读挂载较新版本 fnOS 的 UDS 所在目录 |
| `-e PORT=8086` / `-e PASSWORD=fnosnb` | 见[配置说明](#配置说明) |
| `-p 7777:8086` | 端口映射（宿主机:容器），见[配置说明](#配置说明) |
| `ghcr.io/xxxuuu/fnos-qb-proxy:latest` | 镜像地址与标签，可改为 `v1.0.0` 等固定版本 |

## 配置说明

| 配置项 | 对应参数 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 访问密码 | 环境变量 `PASSWORD` | `fnosnb` | WebUI 访问密码，按需修改 |
| 容器内监听端口 | 环境变量 `PORT` | `8086` | 一般无需修改 |
| 宿主机端口 | 端口映射左侧 | `7777` | 修改后访问 `http://{host}:{端口}` |

Compose 部署时对应修改 YAML 中的 `PASSWORD` 与 `'7777:8086'` 两处即可。

## 验证与排查

部署完成后访问 `http://{host}:7777`（默认密码 `fnosnb`），能看到 qBittorrent WebUI 即成功。

若页面返回 `Proxy Error`，或日志中出现以下错误，可按对应方式排查：

| 错误信息 | 原因与处理 |
| --- | --- |
| `qbt.sock: connect: no such file or directory`（宿主机上 sock 存在） | UDS 所在目录未挂载进容器（如 fnOS 升级后 socket 移到了 `/usr/trim/var/downloadcenter` 但未挂载该目录），检查两处 `-v` 挂载 |
| `qbt.sock: connect: no such file or directory`（宿主机上 sock 也不存在） | fnOS 下载中心（`dlcenter`）在一段时间无人使用后会自动停止，qBittorrent 以 `--stop-with-process` 跟随其退出，sock 随之消失。重新打开下载中心即可恢复；在 WebUI 中保留几个始终做种的 BT 任务可保活，详见 [issue #14](https://github.com/xxxuuu/fnos-qb-proxy/issues/14) |
| `qbittorrent-nox process not found` | 宿主机上 fnOS 下载器未运行（或已自动退出，同上），或容器未配置 `--pid host` |

## 自行构建镜像

在有 Docker 环境的机器上 clone 项目，运行：

```bash
docker build -t fnos-qb-proxy .
```

若通过 Docker Compose 部署，可在 [docker-compose.yml](./docker-compose.yml) 中将 `image` 改为本地构建：

```diff
-    image: ghcr.io/xxxuuu/fnos-qb-proxy:latest
+    build:
+      context: .
+      dockerfile: Dockerfile
```
