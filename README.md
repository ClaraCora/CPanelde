# Corade

> 安装前先按 [安全构建与迁移](SECURITY.md) 完成信任建立。以下本地脚本须来自已审核源码或已验签发行包；公网下载内容不能直接管道交给 root。

Corade is the node agent for the CPanel device management platform. It runs
`sing-box` or `xray-core` nodes assigned by CPanel and synchronizes node
configuration, users, devices, traffic, and machine health.

> This project is for educational and learning purposes only.

## Features

- Protocols: V2Ray family, Trojan, Shadowsocks, Hysteria2, TUIC, and AnyTLS
- Kernels: `sing-box` and `xray-core`, selected per node by CPanel
- Control channel: WebSocket events with cursor-based REST reconciliation
- Dynamic nodes: one agent process starts and stops all nodes assigned to its server
- User controls: speed limits, device limits, alive-IP tracking, and hot updates
- Runtime modes: CPanel device platform or local standalone mode

## Build

```bash
git clone https://github.com/ClaraCora/CPanelde.git
cd CPanelde
make build
```

The repository can also be built directly:

```bash
go build -trimpath -tags "with_quic with_utls with_wireguard with_acme with_clash_api" -o bin/corade ./cmd/corade
```

## One-click installation

The command generated on a CPanel server page contains the required server ID, pinned panel public key,
and a machine-bound one-use enrollment credential (30-minute validity). Public installers and release binaries are hosted in
the CPP distribution repository:

```bash
sudo sh ./install.sh \
  --control-url https://panel.example.com \
  --communication-key ONE_USE_MACHINE_ENROLLMENT_KEY \
  --machine-id mch_example \
  --panel-public-key PINNED_PANEL_PUBLIC_KEY
```

Upgrade an installed Agent without entering its communication key again:

```bash
sudo coradectl upgrade
```

Offline upgrades are a standard release asset starting with v2.0.11. Download
`corade-offline-linux.tar.gz`, `.tar.gz.sha256` and `.tar.gz.sha256.sig` from
[CPP releases](https://github.com/ClaraCora/CPP/releases/tag/corade-latest).
One bundle includes Linux amd64 and arm64, the CLI, signed release metadata,
verification, upgrade and rollback scripts. It supports an existing standard
systemd or OpenRC installation, preserves configuration and identity, and requires
no GitHub access on the target. Verify the archive before extracting; see
[the offline download and upgrade guide](https://github.com/ClaraCora/CPP#agent-离线升级包).
The target still needs access to its configured CPanel for normal operation.

The installer supports Linux amd64 and arm64 with either systemd or Alpine
OpenRC. It authenticates the signed manifest before installing Agent and `coradectl` binaries from the
selected GitHub release. Service startup and failed-upgrade rollback use the
detected init system automatically.

## Service management

`coradectl` provides the same commands on systemd and OpenRC hosts:

```bash
coradectl status
coradectl restart
coradectl logs
```

On systemd, logs remain in the journal. On Alpine/OpenRC, Agent output is stored
in `/var/log/corade/corade.log`; panel-triggered upgrade output is stored in
`/var/log/corade/upgrade.log`. The OpenRC service is enabled in the `default`
runlevel and supervised with automatic restart. An upgrade that does not pass
the service and health checks restores both previous binaries and restarts the
previous Agent.

## Device Platform Configuration

Generate a machine enrollment command in CPanel, expose it through an environment variable, and
point Corade at the public CPanel URL:

```yaml
control:
  mode: "device-platform"
  url: "https://panel.example.com"
  token_env: "CORADE_AGENT_TOKEN"
  machine_id: "mch_example"
  panel_public_key: "PINNED_PANEL_PUBLIC_KEY"

kernel:
  type: "singbox"
  config_dir: "/etc/corade"

log:
  level: "info"
  output: "stdout"
```

```bash
export CORADE_AGENT_TOKEN='one-use-machine-enrollment-key'
./bin/corade -c ./config.yml
```

`CORADE_CONTROL_URL` and `CORADE_AGENT_TOKEN` may also provide the control URL
and token without storing them in YAML.

Corade accepts only `control.mode: device-platform` and `standalone` at runtime.
Legacy panel, node, and machine configurations are rejected by the `corade`
process. Their configuration structures remain temporarily available to
`coradectl` for migration tooling and are not a runtime compatibility mode.

## Standalone Mode

Standalone mode runs one local node without contacting CPanel. It cannot be
combined with `control`, `panel`, `machine`, or `nodes` settings. See the
standalone examples in the configuration tests until a dedicated sample is
added.

## Control Protocol

The Agent control surface is rooted at `/ca/cc`. V2 pins the panel identity, enrolls
with a one-use machine credential, and reconnects with its persisted private key.
Successful enrollment deletes the bootstrap credential. Any explicit legacy
compatibility is restricted by the panel; V2 machines cannot authenticate through it.
Control endpoints require HTTPS/WSS outside numeric loopback addresses.

## Extensions

- Custom routes: [docs-custom-routes.md](docs-custom-routes.md)
- Custom outbounds: [docs-custom-outbounds.md](docs-custom-outbounds.md)
- DNS providers: [docs-dns-providers.md](docs-dns-providers.md)

## License

MPL-2.0.
