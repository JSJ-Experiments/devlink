/*
 * DevLink's Dropbear is a loopback-only, public-key-only management server.
 * Runtime exposure is provided by DevLink reverse WebSocket forwarding.
 */
#define DROPBEAR_SVR_PASSWORD_AUTH 0
#define DROPBEAR_SVR_PAM_AUTH 0
#define DROPBEAR_SVR_PUBKEY_AUTH 1
#define DROPBEAR_SVR_REMOTETCPFWD 0
#define DROPBEAR_SVR_LOCALTCPFWD 1
#define DROPBEAR_X11FWD 0
#define DROPBEAR_SFTPSERVER 0
#define DROPBEAR_SVR_AGENTFWD 0
#define DO_MOTD 0
/* Make bundled CLI/SCP available in root SSH sessions without a system mount. */
#define DEFAULT_ROOT_PATH "/data/adb/modules/devlink/bin:/data/adb/devlink:/system/bin:/system/xbin:/vendor/bin:/bin"
#define DEFAULT_PATH "/system/bin:/system/xbin:/vendor/bin:/bin"
