# CPanelde Agent

> 安装前先按 [安全构建与迁移](SECURITY.md) 完成信任建立。以下本地脚本须来自已审核源码或已验签发行包；公网下载内容不能直接管道交给 root。

CPanelde 是 CPanel 设备管理平台的节点 Agent，负责运行面板分配的 sing-box 或 Xray 节点，并同步配置、用户、设备、流量和服务器状态。

## 一键安装

在 CPanel 后台的服务器页面创建服务器并复制安装命令。完整命令格式如下：

```bash
sudo sh ./install.sh \
  --control-url https://panel.example.com \
  --communication-key ONE_USE_MACHINE_ENROLLMENT_KEY \
  --machine-id mch_example \
  --panel-public-key PINNED_PANEL_PUBLIC_KEY
```

可通过 `--kernel singbox` 或 `--kernel xray` 指定默认内核。节点自身选择的内核会在发布配置后生效。

安装脚本支持 amd64 和 arm64，并自动识别 systemd 或 Alpine OpenRC。Agent 和 `coradectl` 都从指定的 GitHub Release 下载并验证 Ed25519 发布签名、签名版本和 SHA-256 摘要。

## 升级

升级会保留 `/etc/corade/config.yml` 和 `/etc/corade/agent.env`，无需再次提供通讯密钥。新版本未通过服务状态和健康检查时，脚本会恢复 Agent 与 `coradectl` 的旧版本并重新启动。

```bash
sudo coradectl upgrade
```

## 服务管理

systemd 和 Alpine OpenRC 可以统一使用：

```bash
coradectl status
coradectl restart
coradectl logs
```

systemd 日志由 journal 保存。Alpine/OpenRC 的运行日志位于 `/var/log/corade/corade.log`，面板下发的在线升级日志位于 `/var/log/corade/upgrade.log`。OpenRC 服务会加入 `default` 运行级别，并由 `supervise-daemon` 自动拉起。

配置文件位于 `/etc/corade/config.yml`，一次性登记凭据初始保存在权限为 `600` 的 `/etc/corade/agent.env`，成功登记后自动清除；重连使用本机私有身份。

## 手工构建

需要 Go 1.27.3 或以上已修补版本：

```bash
git clone https://github.com/ClaraCora/CPanelde.git
cd CPanelde
go build -trimpath \
  -tags "with_quic with_utls with_wireguard with_acme with_clash_api" \
  -o corade ./cmd/corade
```
