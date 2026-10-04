#!/system/bin/sh
# Borrow Box's stable listener without claiming it. Otherwise own a separate
# loopback listener, preserving pre-existing listen specs and Wireless Debugging.
set -eu
D="${DEVLINK_STATE:-/data/adb/devlink}"
S="$D/adb"
PORT=15556
TAG=devlink-adb
mkdir -p "$S"
loopback_port() {
 ss -ltnp 2>/dev/null | awk '/adbd/ && $4 == "127.0.0.1:5555" {found=1} END {exit !found}'
}
remove_rules() {
 for bin in iptables ip6tables; do
  command -v "$bin" >/dev/null 2>&1 || continue
  while "$bin" -C INPUT ! -i lo -p tcp --dport "$PORT" -m comment --comment "$TAG" -j DROP >/dev/null 2>&1; do
   "$bin" -D INPUT ! -i lo -p tcp --dport "$PORT" -m comment --comment "$TAG" -j DROP || return 1
  done
 done
}
off() {
 # Borrowed Box resources are never stopped, reconfigured, or un-authorized.
 if [ -f "$S/owned" ]; then
  oldboot="$(cat "$S/boot")"
  if [ "$oldboot" = "$(cat /proc/sys/kernel/random/boot_id)" ]; then
   current="$(getprop service.adb.listen_addrs)"
   wanted="$(cat "$S/wanted")"
   if [ "$current" = "$wanted" ]; then
    setprop service.adb.listen_addrs "$(cat "$S/previous")"
    # We never changed service.adb.tcp.port, persist.* or TLS settings.
    stop adbd; start adbd
   else
    # Another module changed the property after us. Do not overwrite it.
    echo 'ADB listener changed by another owner; left its configuration alone' >&2
   fi
  fi
 fi
 remove_rules
 rm -f "$S/owned" "$S/borrowed" "$S/previous" "$S/wanted" "$S/boot"
}
on() {
 # Crash/reboot recovery of our own prior session, before borrowing anything.
 [ ! -f "$S/owned" ] || off
 if [ -s "$D/adbkey.pub" ]; then
  mkdir -p /data/misc/adb
  key="$(cat "$D/adbkey.pub")"
  touch /data/misc/adb/adb_keys
  if ! grep -F -x "$key" /data/misc/adb/adb_keys >/dev/null 2>&1; then
   # Root pre-authorizes only the host's public key; ADB authentication stays on.
   printf '\n%s\n' "$key" >> /data/misc/adb/adb_keys
  fi
  chown 1000:2000 /data/misc/adb/adb_keys
  chmod 0640 /data/misc/adb/adb_keys
  restorecon /data/misc/adb/adb_keys 2>/dev/null || true
 fi
 if loopback_port; then
  echo borrowed > "$S/borrowed"
  echo 5555
  return
 fi
 # Refuse to take over a vendor/non-loopback listener or an occupied own port.
 if ss -ltn 2>/dev/null | awk -v p=":$PORT" '$4 ~ (p "$") {f=1} END {exit !f}'; then
  echo "Port $PORT occupied; refusing to take ownership" >&2;return 1
 fi
 getprop service.adb.listen_addrs > "$S/previous"
 cat /proc/sys/kernel/random/boot_id > "$S/boot"
 prior="$(cat "$S/previous")"
 wanted="tcp:localhost:$PORT"
 if [ -n "$prior" ]; then wanted="$prior,$wanted"; else
  # Preserve an existing tcp.port-based listener while explicitly setting addrs.
  tcp="$(getprop service.adb.tcp.port)"
  case "$tcp" in ''|-1|0) ;; *[!0-9]*) ;; *) wanted="tcp:$tcp,$wanted" ;; esac
 fi
 printf '%s\n' "$wanted" > "$S/wanted"
 touch "$S/owned"
 remove_rules
 iptables -I INPUT 1 ! -i lo -p tcp --dport "$PORT" -m comment --comment "$TAG" -j DROP
 # IPv6 protection is mandatory whenever IPv6 is present, not best-effort.
 if [ -f /proc/net/if_inet6 ]; then
  ip6tables -I INPUT 1 ! -i lo -p tcp --dport "$PORT" -m comment --comment "$TAG" -j DROP
 fi
 setprop service.adb.listen_addrs "$wanted"
 stop adbd; start adbd
 n=0
 while [ "$n" -lt 20 ]; do
  if ss -ltnp 2>/dev/null | awk -v p="127.0.0.1:$PORT" '/adbd/ && $4==p {f=1} END {exit !f}'; then echo "$PORT";return;fi
  sleep 0.2; n=$((n+1))
 done
 echo 'adbd did not bind the requested loopback listener' >&2;return 1
}
case "${1:-}" in on) on ;; off) off ;; *) exit 2 ;; esac
