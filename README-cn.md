# Corade 中文说明

Corade 是一个兼容 Xboard API 的自有品牌节点后端，适用于希望保留上游兼容能力，同时使用独立品牌名称进行部署和运维的场景。

> **声明**：本项目仅供学习与研究用途。

## 项目概览

| 项目 | 说明 |
| --- | --- |
| 定位 | 兼容 Xboard API 的 Corade 节点后端 |
| 内核 | `sing-box`（默认）、`xray-core` |
| 支持协议 | V2Ray 系列、Trojan、Shadowsocks、Hysteria2、TUIC、Naive |
| 工作模式 | 面板下发模式、`standalone` 本地模式 |
| 同步方式 | WebSocket 推送、REST 轮询/上报兜底 |
| 用户控制 | 单用户限速、设备数限制、在线 IP 跟踪 |
| 运行能力 | 热增删改用户 |
| 上报能力 | 流量、在线/活跃 IP、CPU、内存、Swap、磁盘、连接数 |
| 部署方式 | 单 Go 服务、Docker、Docker Compose、systemd |

## 快速开始

### 一条命令安装

使用 [`install.sh`](install.sh) 一键安装节点：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/main/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1
```

如果系统没有 [`curl`](README-cn.md)，可以使用 [`wget`](README-cn.md)：

```bash
bash <(wget -qO- https://raw.githubusercontent.com/ClaraCora/Corade/main/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1
```

常用参数说明：

- [`-a`](install.sh)：面板地址
- [`-t`](install.sh)：面板 Token
- [`-n`](install.sh)：节点 ID
- [`-k`](install.sh)：内核类型，通常为 `singbox` 或 `xray`

指定内核安装示例：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/main/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1 -k xray
```

### 在同一台 VPS 再添加第二个节点

只需要再次执行一次 [`install.sh`](install.sh)，并传入新的 [`node_id`](config.yml.example:61)：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/main/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 2
```

例如：
- 节点 1 是 Shadowsocks
- 节点 2 是 VLESS

只要面板中的节点 ID 不同、监听端口不冲突，就可以在同一台机器同时运行多个 [`corade@<id>`](README-cn.md) 实例。

## 安装方式

### 使用安装脚本

```bash
bash install.sh -a https://panel.example.com -t YOUR_TOKEN -n 1
```

安装脚本默认使用以下运行标识：

- 二进制文件：`corade`
- 配置目录：`/etc/corade`
- systemd 模板服务：`corade@.service`
- Docker 镜像：`ghcr.io/claracora/corade:latest`

### Docker 安装

```bash
docker run -d --restart=always --network=host \
  -e apiHost=https://panel.com \
  -e apiKey=TOKEN \
  -e nodeID=1 \
  ghcr.io/claracora/corade:latest
```

### 原生 / systemd 安装

```bash
make build
sudo cp corade /usr/local/bin/
sudo mkdir -p /etc/corade
sudo cp config.yml.example /etc/corade/config.yml
corade -c /etc/corade/config.yml
```

## 日常运维

### 查看服务状态

查看某个节点实例状态：

```bash
systemctl status corade@1 -l
```

判断节点是否正在运行：

```bash
systemctl is-active corade@1
```

查看所有 Corade 实例：

```bash
systemctl list-units 'corade@*'
```

### 查看日志

查看最近 100 行日志：

```bash
journalctl -u corade@1 -n 100 --no-pager
```

实时跟踪日志：

```bash
journalctl -u corade@1 -f
```

### 启动 / 停止 / 重启

启动节点：

```bash
systemctl start corade@1
```

停止节点：

```bash
systemctl stop corade@1
```

重启节点：

```bash
systemctl restart corade@1
```

设置开机自启：

```bash
systemctl enable corade@1
```

取消开机自启：

```bash
systemctl disable corade@1
```

### 查看监听端口

```bash
ss -lntp | grep corade
```

## 卸载

### 卸载单个节点

如果你安装的 [`install.sh`](install.sh) 支持交互式菜单，可以直接运行：

```bash
bash install.sh
```

然后在菜单中选择卸载。

如果你想手动卸载某个节点，例如节点 1：

```bash
systemctl stop corade@1
systemctl disable corade@1
rm -rf /etc/corade/1
systemctl daemon-reload
```

### 完全卸载整套 Corade

如果你要把整台 VPS 上的 Corade 全部移除：

```bash
systemctl stop 'corade@*'
systemctl disable 'corade@*'
rm -f /etc/systemd/system/corade@.service
rm -f /usr/local/bin/corade
rm -rf /etc/corade
systemctl daemon-reload
```

## 配置说明

示例配置文件见 [`config.yml.example`](config.yml.example)：

```yaml
panel:
  url: "https://panel.com"
  token: "token"
  node_id: 1

kernel:
  type: "singbox"
  config_dir: "/etc/corade"
```

关键字段说明：

- [`panel.url`](config.yml.example:59)：面板地址
- [`panel.token`](config.yml.example:60)：API Token
- [`panel.node_id`](config.yml.example:61)：节点 ID
- [`kernel.type`](config.yml.example:64)：内核类型，通常为 `singbox` 或 `xray`
- [`kernel.config_dir`](config.yml.example:65)：运行时配置目录

## 常见问题排查

### 服务已经启动，但客户端无法连接

先看日志：

```bash
journalctl -u corade@1 -n 100 --no-pager
```

然后依次检查：

- 面板中的协议类型与端口是否正确
- VPS 防火墙是否已放行节点端口
- 是否有其他进程占用了相同端口
- [`node_id`](config.yml.example:61) 是否填错
- 面板 Token 是否有效

### 一台 VPS 部署多个不同类型节点

这是支持的，但必须满足以下条件：

- 每个节点使用不同的 [`node_id`](config.yml.example:61)
- 每个节点使用独立的配置目录，例如 `/etc/corade/1`、`/etc/corade/2`
- 每个节点监听不同端口，不能冲突

例如：
- 节点 1：Shadowsocks
- 节点 2：VLESS

两者可以作为独立的 [`corade@<id>`](README-cn.md) 实例同时运行在同一台机器上。

## 兼容性说明

Corade 保持对 Xboard API 的兼容，但它是一个独立品牌分发版本，不应被表述为官方 Xboard 节点后端。

## 许可证

MPL-2.0.
