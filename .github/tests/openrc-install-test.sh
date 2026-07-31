#!/bin/sh
set -eu

apk add --no-cache busybox-extras curl openrc >/dev/null
mkdir -p /run/openrc /tmp/corade-health
touch /run/openrc/softlevel /tmp/corade-health/healthz

cat >/tmp/corade-good <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-v" ]; then
  echo test-v1
  exit 0
fi
exec httpd -f -p 65530 -h /tmp/corade-health
EOF

cat >/tmp/corade-bad <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-v" ]; then
  echo test-bad
  exit 0
fi
exit 1
EOF
chmod 755 /tmp/corade-good /tmp/corade-bad

/bin/sh /src/install.sh \
  --control-url https://panel.example.com \
  --communication-key aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --machine-id mch_openrc_test \
  --binary /tmp/corade-good

test -x /etc/init.d/corade
rc-update show default | grep -q corade
rc-service --quiet corade status
grep -q 'supervisor="supervise-daemon"' /etc/init.d/corade
test -f /var/log/corade/corade.log

if [ -x /usr/local/bin/coradectl ]; then
  coradectl service status
  coradectl logs -n 1
  coradectl restart
fi

if /bin/sh /src/install.sh upgrade --binary /tmp/corade-bad; then
  echo "upgrade with an unhealthy binary unexpectedly succeeded" >&2
  exit 1
fi

rc-service --quiet corade status
test "$(/usr/local/bin/corade -v)" = "test-v1"
curl --fail --silent http://127.0.0.1:65530/healthz >/dev/null
rc-service corade stop
