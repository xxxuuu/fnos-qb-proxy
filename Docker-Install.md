# Docker 部署说明

## 使用预构建镜像

镜像地址：[ghcr.io/xxxuuu/fnos-qb-proxy](https://github.com/xxxuuu/fnos-qb-proxy/pkgs/container/fnos-qb-proxy)。可用标签示例：`latest`（最新）、`v1.0.0` / `1.0.0`（发版 tag）、或某次提交的 commit 短 ID（用于开发版）。

### 方式 A：Docker Compose + WebUI（推荐）

在 fnOS WebUI → Docker → Compose 中新建项目，项目名称与路径可自定。将项目内的 [docker-compose.yml](./docker-compose.yml) 内容上传或粘贴到 Compose 配置中，保存并启动即可。

默认端口为 `7777`，密码为 `fnosnb`。可在 YAML 中修改 `- PASSWORD=fnosnb` 与 `- '7777:8086'` 两处以更改密码和端口；可在本地编辑后上传，或在 WebUI 中直接修改。

### 方式 B：`docker run`

fnOS WebUI 上运行容器时无法配置除了存储空间以外的挂载点，因此需要通过 ssh 连接到机器上执行命令。纯小白建议使用上方 Docker Compose + WebUI 方式

```bash
docker run -d \
  --name fnOS-qBit-Proxy \
  --pid host \
  -e PORT=8086 \
  -e PASSWORD=fnosnb \
  -p 7777:8086 \
  -v /home:/home:ro \
  ghcr.io/xxxuuu/fnos-qb-proxy:latest
```

| 参数                                  | 说明                                                                            |
| ------------------------------------- | ------------------------------------------------------------------------------- |
| `-d`                                  | 后台运行容器                                                                    |
| `--name fnOS-qBit-Proxy`              | 容器名称                                                                        |
| `--pid host`                          | 使用宿主机 PID namespace，便于代理从 `/proc` 发现 qBittorrent 进程及其 UDS 路径 |
| `-v /home:/home:ro`                   | 只读挂载宿主机 `/home`，使从进程命令行解析出的 `qbt.sock` 路径在容器内可访问    |
| `-e PORT=8086`                        | 容器内代理监听端口，默认8086                                                    |
| `-e PASSWORD=fnosnb`                  | WebUI 访问密码；默认fnosnb，按需修改                                            |
| `-p 7777:8086`                        | 端口映射（宿主机:容器），按需修改                                               |
| `ghcr.io/xxxuuu/fnos-qb-proxy:latest` | 镜像地址与标签，可改为 `v1.0.0` 等固定版本                                      |

## 自行构建镜像

使用项目中的 [Dockerfile](./Dockerfile) 在本地构建镜像。需已安装 Docker。

在有 Docker 环境的机器上 clone 项目，运行：

```bash
docker build -t fnos-qb-proxy .
```

若通过 Docker Compose 部署，可改为从本地构建而非拉取预构建镜像，在 [docker-compose.yml](./docker-compose.yml) 中将 `image` 改为 `build`：

```diff
-    image: ghcr.io/xxxuuu/fnos-qb-proxy:latest
+    build:
+      context: .
+      dockerfile: Dockerfile
```
