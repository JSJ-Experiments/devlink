#!/system/bin/sh
SKIPUNZIP=0
ui_print '- DevLink: generic SSH/ADB reverse tunnel'
case "$ARCH" in arm64|arm|x64|x86) ;; *) abort "Unsupported architecture: $ARCH" ;; esac
mv "$MODPATH/bin/devlink-$ARCH" "$MODPATH/bin/devlink"
for b in dropbear dropbearkey scp; do
 mv "$MODPATH/bin/$b-$ARCH" "$MODPATH/bin/$b" || abort "Missing $b for $ARCH"
done
rm -f "$MODPATH"/bin/*-arm "$MODPATH"/bin/*-arm64 "$MODPATH"/bin/*-x64 "$MODPATH"/bin/*-x86
set_perm_recursive "$MODPATH/bin" 0 0 0700 0700
set_perm_recursive "$MODPATH/scripts" 0 0 0700 0700
for f in service.sh action.sh uninstall.sh; do set_perm "$MODPATH/$f" 0 0 0755; done
mkdir -p /data/adb/devlink
chmod 0700 /data/adb/devlink
cat > /data/adb/devlink/devlink <<'SH'
#!/system/bin/sh
export DEVLINK_MODULE=/data/adb/modules/devlink
exec /data/adb/modules/devlink/bin/devlink "$@"
SH
chmod 0700 /data/adb/devlink/devlink
# First installation is ready to use for two hours after first boot, then off.
# Upgrades preserve the user's on/off decision and existing deadline.
if [ ! -f /data/adb/devlink/installed ]; then
 DEVLINK_MODULE="$MODPATH" "$MODPATH/bin/devlink" init || abort 'Cannot initialize DevLink'
 touch /data/adb/devlink/first-boot /data/adb/devlink/installed
fi
ui_print '- First activation: auto-enroll and enable for 2h, then turn off'
ui_print '- CLI: su -c "/data/adb/devlink/devlink on 2h"'
ui_print '- Open WebUI for toggles, status and endpoint settings'

# Activation waits for this install shell to exit, so KernelSU can finish WebUI
# contexts/update metadata before its staging directory is promoted.
if [ "$(getprop sys.boot_completed)" = 1 ]; then
 DEVLINK_MODULE="$MODPATH" "$MODPATH/bin/devlink" activate "$MODPATH" "$$" || ui_print '- Hot activation unavailable; run devlink reload'
 ui_print '- Hot activation scheduled: no reboot required'
fi
