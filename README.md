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

## Install

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

### Multi-node installer
```bash
bash install.sh -a https://panel.example.com -t YOUR_TOKEN -n 1
```

The installer uses these runtime identities by default:

- Binary: `corade`
- Config directory: `/etc/corade`
- systemd template: `corade@.service`
- Docker image: `ghcr.io/claracora/corade:latest`

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

## Compatibility Notes

Corade keeps compatibility with the Xboard API, but it is an independently branded distribution and should not be presented as the official Xboard node backend.

## License

MPL-2.0.
