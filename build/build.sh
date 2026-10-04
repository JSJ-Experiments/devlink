#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
: "${NDK:?Set NDK to the Android NDK root}"
ARCHES=${ARCHES:-'arm64 arm x64 x86'}
VERSION=${VERSION:-0.1.0}
VERSION_CODE=${VERSION_CODE:-1}
ENDPOINT=${ENDPOINT:-https://cfarm.jadenjsj.com/devlink}
FINGERPRINT=${FINGERPRINT:-$(python3 -c 'import json;print(json.load(open("deploy/public.json"))["fingerprint"])')}
[[ "$ENDPOINT" != *' '* && "$FINGERPRINT" != *' '* ]] || { echo 'Build values cannot contain spaces' >&2; exit 1; }
TOOLCHAIN="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin"
mkdir -p .cache module/bin module/licenses dist
url=https://matt.ucc.asn.au/dropbear/releases/dropbear-2026.94.tar.bz2
[[ -f .cache/dropbear.tar.bz2 ]] || curl -fL --retry 3 "$url" -o .cache/dropbear.tar.bz2
echo 'e098034a843699200c8c977a991fff73159735bf795d5f72ef672c41a6b1ae81  .cache/dropbear.tar.bz2' | sha256sum -c -
for arch in $ARCHES; do
 case "$arch" in
 arm64) triple=aarch64-linux-android; goarch=arm64; api=24 ;;
 arm) triple=armv7a-linux-androideabi; goarch=arm; api=24 ;;
 x64) triple=x86_64-linux-android; goarch=amd64; api=24 ;;
 x86) triple=i686-linux-android; goarch=386; api=24 ;;
 *) echo "Unknown architecture: $arch" >&2; exit 1 ;;
 esac
 export CC="$TOOLCHAIN/$triple$api-clang" AR="$TOOLCHAIN/llvm-ar" RANLIB="$TOOLCHAIN/llvm-ranlib" STRIP="$TOOLCHAIN/llvm-strip"
 src="$PWD/.cache/dropbear-$arch"
 if [[ ! -f "$src/.patched" ]]; then
  rm -rf "$src";mkdir -p "$src";tar -xjf .cache/dropbear.tar.bz2 --strip-components=1 -C "$src"
  cp build/dropbear-localoptions.h "$src/localoptions.h"
  patch -d "$src" -p1 < build/dropbear-android-authorized-keys.patch
  touch "$src/.patched"
 fi
 (
  cd "$src"
  export CFLAGS='-Os -fPIE' LDFLAGS='-Wl,-z,relro,-z,now -pie'
  ./configure --host="$triple" --disable-zlib --disable-syslog --disable-shadow --disable-lastlog --disable-utmp --disable-utmpx --disable-wtmp --disable-wtmpx > configure-build.log
  make -j"${JOBS:-$(nproc)}" PROGRAMS='dropbear dropbearkey scp' > make-build.log
  "$STRIP" dropbear dropbearkey scp
 )
 for b in dropbear dropbearkey scp; do cp "$src/$b" "module/bin/$b-$arch";done
 cp "$src/LICENSE" module/licenses/dropbear-LICENSE
 # Use Bionic/cgo, not a static Linux build: Android's DNS resolution needs
 # libc/netd to follow Wi-Fi/mobile network changes instead of resolv.conf.
 CGO_ENABLED=1 GOOS=android GOARCH="$goarch" GOARM=7 go build -buildmode=pie -trimpath \
  -ldflags "-s -w -X main.defaultEndpoint=$ENDPOINT -X main.defaultFingerprint=$FINGERPRINT -X main.version=$VERSION" \
  -o "module/bin/devlink-$arch" ./cmd/devlink
 done
npm ci
npm run build
cp "$(go env GOPATH)/pkg/mod/github.com/jpillora/chisel@v1.12.0/LICENSE" module/licenses/chisel-LICENSE
rm -rf .staging;mkdir .staging
cp -a module/. .staging/
sed -i "s/^version=.*/version=$VERSION/;s/^versionCode=.*/versionCode=$VERSION_CODE/" .staging/module.prop
(cd .staging && zip -qr -9 -X "../dist/devlink-$VERSION.zip" .)
sha256sum "dist/devlink-$VERSION.zip" > "dist/devlink-$VERSION.zip.sha256"
echo "Built dist/devlink-$VERSION.zip"
