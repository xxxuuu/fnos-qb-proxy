# Docker Install

首先进行一些配置，如下。配置过程中将多次用到查找与替换的功能，建议使用支持相应功能的编辑器而非手动替换，以免出错。此外，除非您清楚地知道您在做什么，否则不建议修改其他位置的内容。

## 修改Dockerfile

请将如下字段中的`fnos-qb-proxy_linux-amd64`改为您下载的可执行文件的文件名：

```
COPY fnos-qb-proxy_linux-amd64 /usr/local/bin/fnos-qb-proxy
```

## 修改docker-compose.yml

如果您需要自定义代理容器在宿主机上暴露的端口号（效果相当于上文中通过`--port`参数传入的端口号），请修改如下字段中引号左边的`7777`为您所需要的端口号：
```
    ports:
      - "7777:8086"
```

如果您需要自定义qBittorrent WebUI的访问密码（效果相当于上文中通过`-p`或`--password`参数传入的密码），请修改如下字段中的`fnosnb`为您所需要的密码，注意，此处请勿将密码修改为非ASCII字符，否则qBittorrent的WebUI将错误转译密码导致登录失败：

```
    environment:
      - PASSWORD=fnosnb
```

⚠️ 只要您的用户名不是`Kainichy`，您必须将以下内容中的`Kainichy`替换为您的用户名：
```
    volumes:
      - /home/Kainichy/qbt.sock:/home/Kainichy/qbt.sock
```

然后开始构建并启动，如下。

## 构建镜像，启动容器

有两种方法完成这件事。

### SSH

SSH访问您的主机，然后在含有`Dockerfile`、`docker-compose.yml`以及可执行文件的目录下执行`docker compose up -d`。

### fnOS WebUI / 飞牛OS网页版

1. 创建一个目录，并将`Dockerfile`以及可执行文件通过您喜爱的方式上传到您的飞牛，注意此时不要上传`docker-compose.yml`，否则可能会出现错误。并且请注意，目录需要上传到一个您接下来操作的账户能够访问的目录下。
2. 登录您的飞牛OS网页版，进入Docker应用，在边栏中选择Compose，选择`新建项目`
3. 填写`项目名称`，选择第一步中上传的文件夹，此时在对话框中上传`docker-compose.yml`，或者复制并粘贴`docker-compose.yml`的全部内容，注意不要打乱格式。
4. 选择`确定`

此时您的容器应该正常运行，并且您将会在`7777`或您指定的端口号上访问飞牛自带的`trim-qbittorrent`的WebUI。

## 确保下载中心服务在Docker服务之前启动

我们注意到在fnOS中，Docker服务可能在下载中心启动之前完成启动，导致`~/qbt.sock`被Docker意外占用，进而导致下载中心携带的的qBittorrent进程无法正常启动。因此，我们需要确保下载中心服务在Docker服务之前启动，具体方法为如下。

在终端/SSH中运行`sudo systemctl edit docker.service`，默认情况下，这将打开一个由Nano编辑器承载的`/etc/systemd/system/docker.service.d/override.conf`编辑窗口，此时在

```
### Anything between here and the comment below will become the new contents of the file
```
（从1开始数，通常位于第2行）

以及

```
### Lines below this comment will be discarded
```
（通常位于第6行）

这两行注释（通常显示为蓝色）之间添加如下内容：
```
[Unit]
After=dlcenter.service
```

确保前几行看起来像是：
```
### Editing /etc/systemd/system/docker.service.d/override.conf
### Anything between here and the comment below will become the new contents of the file

[Unit]
After=dlcenter.service

### Lines below this comment will be discarded

### /etc/systemd/system/docker.service
```

这将使`dlcenter.service`（fnOS的下载中心服务，亦负责启动自带的qBittorrent）在`docker.service`之前运行。

最后，运行`sudo systemctl daemon-reload`来使得这些修改生效。