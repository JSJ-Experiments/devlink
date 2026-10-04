#!/system/bin/sh
# Plain-script/no-system-mount module: promote a completed KernelSU install and
# restart only DevLink. No reboot, global ksud service replay, or Box restart.
set -eu
SRC="${1:-${0%/scripts/*}}"
INSTALLER="${2:-0}"
ADB="${DEVLINK_ADB:-/data/adb}"
ACTIVE="$ADB/modules/devlink"
D="${DEVLINK_STATE:-$ADB/devlink}"
SHELL_BIN="${DEVLINK_SHELL:-/system/bin/sh}"
[ "$(id -u)" = 0 ] || { echo 'Root required' >&2; exit 1; }
mkdir -p "$D"
if ! mkdir "$D/hotreload.lock" 2>/dev/null; then
 old="$(cat "$D/hotreload.lock/pid" 2>/dev/null || true)"
 if [ -n "$old" ] && kill -0 "$old" 2>/dev/null; then echo 'Hot reload already running'; exit 0; fi
 rm -rf "$D/hotreload.lock"
 mkdir "$D/hotreload.lock"
fi
echo $$ > "$D/hotreload.lock/pid"
trap 'rm -rf "$D/hotreload.lock"' EXIT
# The installer applies WebUI permissions/context and creates its update marks
# after customize.sh returns. Do not rename its staging directory prematurely.
if [ "$INSTALLER" -gt 1 ]; then
 n=0
 while kill -0 "$INSTALLER" 2>/dev/null; do
  [ "$n" -lt 60 ] || { echo 'Installer still busy; use devlink reload later' >&2; exit 1; }
  sleep 1; n=$((n+1))
 done
fi
[ "$(sed -n 's/^id=//p' "$SRC/module.prop")" = devlink ] || { echo 'Not a DevLink module' >&2; exit 1; }
[ ! -d "$SRC/system" ] || { echo 'Refusing to hot-reload a system-mount module' >&2; exit 1; }
[ -x "$SRC/bin/devlink" ] || { echo 'Device binary missing' >&2; exit 1; }
export DEVLINK_STATE="$D"
# Stop runtime only: preserve enabled/disabled, expiry, identity and preferences.
"$SRC/bin/devlink" stop-runtime
BACKUP="$D/module.previous"
if [ "$SRC" != "$ACTIVE" ]; then
 rm -rf "$BACKUP"
 if [ -d "$ACTIVE" ]; then
  [ "$(sed -n 's/^id=//p' "$ACTIVE/module.prop")" = devlink ] || exit 1
  mv "$ACTIVE" "$BACKUP"
 fi
 if ! mv "$SRC" "$ACTIVE"; then
  [ ! -d "$BACKUP" ] || mv "$BACKUP" "$ACTIVE"
  exit 1
 fi
 [ ! -f "$BACKUP/disable" ] || touch "$ACTIVE/disable"
fi
rm -f "$ACTIVE/update"
export DEVLINK_MODULE="$ACTIVE"
if [ -f "$ACTIVE/disable" ] || [ -f "$ACTIVE/remove" ]; then
 echo 'DevLink updated; module remains disabled'
else
 if ! "$SHELL_BIN" "$ACTIVE/service.sh"; then
  echo 'Activation failed; restoring previous module' >&2
  if [ -d "$BACKUP" ]; then
   mv "$ACTIVE" "$D/module.failed"
   mv "$BACKUP" "$ACTIVE"
   "$SHELL_BIN" "$ACTIVE/service.sh" || true
  fi
  exit 1
 fi
 echo 'DevLink hot-reloaded without reboot'
fi
rm -rf "$BACKUP"
