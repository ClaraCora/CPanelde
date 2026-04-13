# Corade

Corade is a self-branded node backend compatible with the Xboard API. It is designed for operators who want an independently branded runtime while preserving upstream compatibility.

> **Disclaimer**: This project is for educational and learning purposes only.

## Overview

| Item | Description |
| --- | --- |
| Role | Corade-branded node backend with Xboard API compatibility |
| Kernels | `sing-box` (default), `xray-core` |
| Protocols | V2Ray family, Trojan, Shadowsocks, Hysteria2, TUIC, Naive |
| Modes | Panel-managed mode, `standalone` mode |
| Sync | WebSocket push, REST polling/report fallback |
| User controls | Per-user speed limit, device limit, alive IP tracking |
| Runtime ops | Hot user add/remove/update |
| Reporting | Traffic, online/alive-IP state, CPU, memory, swap, disk, connection count |
| Deployment | Single Go service, Docker, Docker Compose, systemd |

## Quick Start

### One-line install

Install a node directly with [`install.sh`](install.sh):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/dev/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1
```

If [`curl`](README.md) is unavailable, use [`wget`](README.md):

```bash
bash <(wget -qO- https://raw.githubusercontent.com/ClaraCora/Corade/dev/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1
```

Common parameters:

- [`-a`](install.sh): panel URL
- [`-t`](install.sh): panel token
- [`-n`](install.sh): node ID
- [`-k`](install.sh): kernel type, usually `singbox` or `xray`

Example with explicit kernel:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/dev/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 1 -k xray
```

### Add a second node on the same VPS

Run [`install.sh`](install.sh) again with another [`node_id`](config.yml.example:61):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/dev/install.sh) -a https://panel.example.com -t YOUR_TOKEN -n 2
```

This will create another instance such as [`corade@2`](README.md) while keeping [`corade@1`](README.md) running.

### Time sync quick fix

If SS2022 logs show [`bad timestamp`](README.md:241), run the time sync helper script first:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ClaraCora/Corade/dev/time-sync-check.sh)
```

If [`curl`](README.md) is unavailable:

```bash
bash <(wget -qO- https://raw.githubusercontent.com/ClaraCora/Corade/dev/time-sync-check.sh)
```

## Install Methods

### Multi-node installer

```bash
bash install.sh -a https://panel.example.com -t YOUR_TOKEN -n 1
```

The installer uses these runtime identities by default:

- Binary: `corade`
- Config directory: `/etc/corade`
- systemd template: `corade@.service`
- Docker image: `ghcr.io/claracora/corade:latest`

### Docker

```bash
docker run -d --restart=always --network=host \
  -e apiHost=https://panel.com \
  -e apiKey=TOKEN \
  -e nodeID=1 \
  ghcr.io/claracora/corade:latest
```

### Native / systemd

```bash
make build
sudo cp corade /usr/local/bin/
sudo mkdir -p /etc/corade
sudo cp config.yml.example /etc/corade/config.yml
corade -c /etc/corade/config.yml
```

## Daily Operations

### Service status

Check a node status:

```bash
systemctl status corade@1 -l
```

Check whether it is running:

```bash
systemctl is-active corade@1
```

Check all Corade instances:

```bash
systemctl list-units 'corade@*'
```

### Logs

View recent logs:

```bash
journalctl -u corade@1 -n 100 --no-pager
```

Follow logs in real time:

```bash
journalctl -u corade@1 -f
```

### Restart / stop / start

Restart a node:

```bash
systemctl restart corade@1
```

Stop a node:

```bash
systemctl stop corade@1
```

Start a node:

```bash
systemctl start corade@1
```

Enable auto-start on boot:

```bash
systemctl enable corade@1
```

Disable auto-start on boot:

```bash
systemctl disable corade@1
```

### Check listening ports

```bash
ss -lntp | grep corade
```

### Update all nodes

Update the binary and restart all running nodes:

```bash
bash install.sh update
```

### Remove a specific node

Remove only node 2:

```bash
bash install.sh remove 2
```

## Uninstall

### Uninstall one node

If your installed [`install.sh`](install.sh) supports interactive uninstall, run:

```bash
bash install.sh
```

and choose the uninstall option.

Or remove the instance manually:

```bash
systemctl stop corade@1
systemctl disable corade@1
rm -rf /etc/corade/1
systemctl daemon-reload
```

### Uninstall all nodes and binary

If you want to fully remove Corade from the VPS:

```bash
systemctl stop 'corade@*'
systemctl disable 'corade@*'
rm -f /etc/systemd/system/corade@.service
rm -f /usr/local/bin/corade
rm -rf /etc/corade
systemctl daemon-reload
```

## Configuration

Example [`config.yml`](config.yml.example):

```yaml
panel:
  url: "https://panel.com"
  token: "token"
  node_id: 1

kernel:
  type: "singbox"
  config_dir: "/etc/corade"
```

Important fields:

- [`panel.url`](config.yml.example:59): panel address
- [`panel.token`](config.yml.example:60): API token
- [`panel.node_id`](config.yml.example:61): node ID from panel
- [`kernel.type`](config.yml.example:64): `singbox` or `xray`
- [`kernel.config_dir`](config.yml.example:65): runtime config directory

## Troubleshooting

### Service starts but users cannot connect

Check logs first:

```bash
journalctl -u corade@1 -n 100 --no-pager
```

Then verify:

- The panel node uses the correct protocol and port
- The VPS firewall allows the node port
- Another process is not already listening on the same port
- The panel token and [`node_id`](config.yml.example:61) are correct

### Add multiple protocol types on one VPS

This is supported. The requirements are:

- each node uses a different [`node_id`](config.yml.example:61)
- each node has its own config path like `/etc/corade/1` and `/etc/corade/2`
- each node listens on a different port

Example:

- node 1: Shadowsocks
- node 2: VLESS

Both can run together on the same server as separate [`corade@<id>`](README.md) instances.

## Compatibility Notes

Corade keeps compatibility with the Xboard API, but it is an independently branded distribution and should not be presented as the official Xboard node backend.

## License

MPL-2.0.
