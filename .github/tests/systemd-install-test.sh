#!/bin/sh
set -eu

apk add --no-cache curl >/dev/null
mkdir -p /run/systemd/system

cat >/usr/local/bin/systemctl <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>/tmp/systemctl.commands
exit 0
EOF
cat >/usr/local/bin/journalctl <<'EOF'
#!/bin/sh
exit 0
EOF
cat >/tmp/corade-good <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-v" ]; then
  echo test-systemd
  exit 0
fi
exit 0
EOF
chmod 755 /usr/local/bin/systemctl /usr/local/bin/journalctl /tmp/corade-good

/bin/sh /src/install.sh \
  --control-url https://panel.example.com \
  --communication-key aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --machine-id mch_systemd_test \
  --health-port 0 \
  --binary /tmp/corade-good

test -f /etc/systemd/system/corade.service
grep -q 'EnvironmentFile=/etc/corade/agent.env' /etc/systemd/system/corade.service
grep -q '^enable corade.service$' /tmp/systemctl.commands
grep -q '^restart corade.service$' /tmp/systemctl.commands
