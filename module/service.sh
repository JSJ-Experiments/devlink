#!/system/bin/sh
MODDIR=${0%/*}
# No background shell/watchdog: the Go client owns reconnect and expiration.
while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 2; done
export DEVLINK_MODULE="$MODDIR"
# Reset orphaned ADB state after an unclean exit/reboot, without touching Box.
[ ! -f /data/adb/devlink/adb/owned ] || /system/bin/sh "$MODDIR/scripts/adb.sh" off
if [ -f /data/adb/devlink/first-boot ]; then
 rm -f /data/adb/devlink/first-boot
 "$MODDIR/bin/devlink" on 2h
else
 "$MODDIR/bin/devlink" boot
fi
