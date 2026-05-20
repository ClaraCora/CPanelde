# Corade

Corade is a self-branded node backend compatible with the Xboard API. It supports `sing-box` / `xray-core` dual kernels.

> **Disclaimer**: This project is for educational and learning purposes only.

## Features

- Protocols: V2Ray family, Trojan, Shadowsocks, Hysteria2, TUIC, AnyTLS
- Sync: WebSocket push + REST polling dual channel
- User controls: speed limit, device limit, alive-IP tracking, hot update
- Deploy modes: node mode, machine mode, standalone mode
- Multi-instance: single process binding multiple panels / nodes

## Install

### Build from source

```bash
git clone https://github.com/ClaraCora/coradem.git
cd coradem
make build
```

### Installer (Linux systemd)

```bash
# Node mode
curl -fsSL https://raw.githubusercontent.com/ClaraCora/coradem/main/install.sh | \
  sudo bash -s -- --mode node --panel https://panel.example.com --token TOKEN --node-id 1

# Machine mode
curl -fsSL https://raw.githubusercontent.com/ClaraCora/coradem/main/install.sh | \
  sudo bash -s -- --mode machine --panel https://panel.example.com --token TOKEN --machine-id 1
```

### Upgrade / migrate from the old repository

If a VPS was installed from the old repository, use the new installer URL once. It preserves `/etc/corade/config.yml`, replaces `corade` / `coradectl`, restarts `corade.service`, and switches future upgrades to this repository.

```bash
curl -fsSL https://raw.githubusercontent.com/ClaraCora/coradem/main/install.sh | sudo bash -s -- upgrade
```

After that, regular upgrades can use:

```bash
sudo coradectl upgrade
```

## coradectl

Run `coradectl` after installation for help. Common commands:

```bash
sudo coradectl upgrade                  # update Corade on the VPS
coradectl list                          # list all instances
coradectl status                        # running status
coradectl bind add-node --panel URL --token TOKEN --node-id 1
coradectl bind add-machine --panel URL --token TOKEN --machine-id 1
coradectl bind remove-node --panel URL --node-id 1
coradectl service restart
```

## Configuration

Legacy single-panel config is fully compatible. Appending bindings auto-migrates to `instances` format. See `config.yml.example`.

## Extensions

- Custom routes: [docs-custom-routes.md](docs-custom-routes.md)
- Custom outbounds: [docs-custom-outbounds.md](docs-custom-outbounds.md)
- DNS providers (ACME DNS-01): [docs-dns-providers.md](docs-dns-providers.md)

## License

MPL-2.0.
