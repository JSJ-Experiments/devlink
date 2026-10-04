#!/usr/bin/env bash
# Provision the loopback origin. Caddy routing is explicit/separate, so this
# installer never rewrites an unrelated site's configuration.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $(id -u) -eq 0 ]] || { echo 'Run as root, with SSH_KEYS pointing to operator public keys.' >&2; exit 1; }
: "${SSH_KEYS:?Set SSH_KEYS to an OpenSSH authorized_keys/public-key file}"
PREFIX=${PREFIX:-/devlink}
[[ "$PREFIX" == /* && "$PREFIX" != *' '* && "$PREFIX" != *'..'* && "$PREFIX" != / ]] || { echo 'Invalid PREFIX' >&2; exit 1; }
getent passwd devlink >/dev/null || useradd --system --home-dir /var/lib/devlink --shell /usr/sbin/nologin devlink
install -d -m 0755 /usr/local/lib/devlink
install -d -m 0750 -o root -g devlink /etc/devlink
if [[ -z ${SERVER_BINARY:-} && -f ./devlink-server ]]; then SERVER_BINARY="$PWD/devlink-server"; fi
if [[ -n ${SERVER_BINARY:-} ]]; then
 install -m 0755 "$SERVER_BINARY" /usr/local/lib/devlink/devlink-server
else
 go build -trimpath -ldflags '-s -w' -o .cache-devlink-server ./cmd/devlink-server
 install -m 0755 .cache-devlink-server /usr/local/lib/devlink/devlink-server
 unlink .cache-devlink-server
fi
install -m 0644 "$SSH_KEYS" /etc/devlink/authorized_keys
[[ -z ${ADB_KEY:-} ]] || install -m 0644 "$ADB_KEY" /etc/devlink/adbkey.pub
sed "s#^ExecStart=.*#ExecStart=/usr/local/lib/devlink/devlink-server --path $PREFIX#" deploy/devlink-server.service > /etc/systemd/system/devlink-server.service
CLI=tools/devlink
[[ ! -f ./devlink ]] || CLI=./devlink
install -m 0755 "$CLI" /usr/local/bin/devlink
ln -sfn /usr/local/lib/devlink/devlink-server /usr/local/bin/devlink-server
systemctl daemon-reload
systemctl enable devlink-server
systemctl restart devlink-server
printf 'Add deploy/Caddyfile.fragment to your hostname, replacing /devlink with %s.\n' "$PREFIX"
printf 'Then validate and reload Caddy. Check /%s/health.\n' "${PREFIX#/}"
printf 'Copy the fingerprint from journalctl -u devlink-server into deploy/public.json before building modules.\n'
