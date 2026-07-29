# Corade

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

The command generated on a CPanel server page contains the required server ID
and communication key. It uses this repository's installer directly:

```bash
curl -fsSL https://raw.githubusercontent.com/ClaraCora/CPanelde/main/install.sh | sudo bash -s -- \
  --control-url https://panel.example.com \
  --communication-key YOUR_COMMUNICATION_KEY \
  --machine-id mch_example
```

Upgrade an installed Agent without entering its communication key again:

```bash
curl -fsSL https://raw.githubusercontent.com/ClaraCora/CPanelde/main/install.sh | sudo bash -s -- upgrade
```

The installer supports Linux amd64 and arm64 with systemd. It first downloads a
checksummed binary from CPanel and falls back to the checksummed `latest` GitHub
release when the panel artifact is unavailable.

## Device Platform Configuration

Create an Agent token in CPanel, expose it through an environment variable, and
point Corade at the public CPanel URL:

```yaml
control:
  mode: "device-platform"
  url: "https://panel.example.com"
  token_env: "CORADE_AGENT_TOKEN"

kernel:
  type: "singbox"
  config_dir: "/etc/corade"

log:
  level: "info"
  output: "stdout"
```

```bash
export CORADE_AGENT_TOKEN='agent-token-from-cpanel'
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

The Agent control surface is rooted at `/ca/cc`. Corade authenticates only with
`Authorization: Bearer <agent-token>` and protocol version `1.0`; it does not
send machine IDs, node types, or tokens in URLs.

## Extensions

- Custom routes: [docs-custom-routes.md](docs-custom-routes.md)
- Custom outbounds: [docs-custom-outbounds.md](docs-custom-outbounds.md)
- DNS providers: [docs-dns-providers.md](docs-dns-providers.md)

## License

MPL-2.0.
