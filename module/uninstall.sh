#!/system/bin/sh
MODDIR=${0%/*}
export DEVLINK_MODULE="$MODDIR"
"$MODDIR/bin/devlink" off
/system/bin/sh "$MODDIR/scripts/adb.sh" off
# Keep device identity/host keys for reinstall. Explicitly delete
# /data/adb/devlink if a fresh identity is desired.
rm -f /data/adb/devlink/devlink
