#!/usr/bin/env bash
# Build self-contained origin bundles. No private state or device credentials.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=${OUT:-dist/release}
mkdir -p "$OUT"
stage=$(mktemp -d)
trap 'rm -r "$stage"' EXIT
for arch in ${ORIGIN_ARCHES:-amd64 arm64}; do
 case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1;; esac
 root="$stage/devlink-origin-linux-$arch"
 mkdir -p "$root/deploy"
 CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags '-s -w' -o "$root/devlink-server" ./cmd/devlink-server
 install -m 0755 tools/devlink "$root/devlink"
 install -m 0755 deploy/install-server.sh "$root/deploy/install-server.sh"
 cp deploy/devlink-server.service deploy/Caddyfile.fragment deploy/public.json "$root/deploy/"
 cp README.md LICENSE "$root/"
 tar -czf "$OUT/devlink-origin-linux-$arch.tar.gz" -C "$stage" "devlink-origin-linux-$arch"
done
