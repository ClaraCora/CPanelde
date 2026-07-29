# CPanelde Agent

CPanelde 是 CPanel 设备管理平台的节点 Agent，负责运行面板分配的 sing-box 或 Xray 节点，并同步配置、用户、设备、流量和服务器状态。

## 一键安装

在 CPanel 后台的服务器页面创建服务器并复制安装命令。完整命令格式如下：

```bash
curl -fsSL https://raw.githubusercontent.com/ClaraCora/CPanelde/main/install.sh | sudo bash -s -- \
  --control-url https://panel.example.com \
  --communication-key YOUR_COMMUNICATION_KEY \
  --machine-id mch_example
```

可通过 `--kernel singbox` 或 `--kernel xray` 指定默认内核。节点自身选择的内核会在发布配置后生效。

安装脚本支持 systemd 的 Linux amd64 和 arm64。脚本优先从 CPanel 下载带 SHA-256 校验的 Agent；面板文件不可用时，会自动回退到本仓库的 `latest` Release。

## 升级

升级会保留 `/etc/corade/config.yml` 和 `/etc/corade/agent.env`，无需再次提供通讯密钥。新版本无法正常启动时，脚本会自动恢复先前的二进制。

```bash
curl -fsSL https://raw.githubusercontent.com/ClaraCora/CPanelde/main/install.sh | sudo bash -s -- upgrade
```

## 服务管理

```bash
systemctl status corade
journalctl -u corade -f
systemctl restart corade
```

配置文件位于 `/etc/corade/config.yml`，通讯密钥单独保存在权限为 `600` 的 `/etc/corade/agent.env`。

## 手工构建

需要 Go 1.26 或以上版本：

```bash
git clone https://github.com/ClaraCora/CPanelde.git
cd CPanelde
go build -trimpath \
  -tags "with_quic with_utls with_wireguard with_acme with_clash_api" \
  -o corade ./cmd/corade
```
