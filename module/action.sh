#!/system/bin/sh
MODDIR=${0%/*}
export DEVLINK_MODULE="$MODDIR"
if "$MODDIR/bin/devlink" status --json | grep -q '"enabled":true'; then
 "$MODDIR/bin/devlink" off
else
 "$MODDIR/bin/devlink" on 2h
fi
"$MODDIR/bin/devlink" status
