# DevLink

Generic, on-demand rooted Android **SSH + optional ADB** over a reverse HTTPS/WebSocket connection. KernelSU / KernelSU Next module; no Chisel fork and no Tailscale dependency. Runs on arm64, ARMv7, x86-64 and x86 (Android API 24+).

## Install and connect

Install **[devlink.zip](https://github.com/JSJ-Experiments/devlink/releases/download/latest/devlink.zip)** in KernelSU. On a running/hot-rooted device it **activates immediately, without reboot**. First installation automatically enrolls and enables **SSH for two hours**, then turns itself off. No passwords, tokens, or keys need to be entered on the device. ADB is initially off. Upgrades preserve state.

Open the module's **WebUI** to enable permanently, enable with a timer, disable, toggle ADB, or change the origin/subpath. The KernelSU **Action** button toggles off / on for two hours.

On the origin server:

```sh
devlink-devices --connected
# Read that device's allocated SSH/ADB ports, then:
ssh root@127.0.0.1 -p 18622
adb connect 127.0.0.1:18623  # only after enabling DevLink ADB
```

Ports are fixed per installation and survive upgrades, reboot and endpoint changes; **do not assume every device uses 18622**. Device names are Android model labels, not hard-coded personal/device names. Enrollment secrets and host keys survive upgrades/reboots/reinstall in `/data/adb/devlink`.

The build in this repository defaults to `https://cfarm.jadenjsj.com/devlink` and pins that deployment's Chisel server fingerprint. If deploying your own origin, build with your own endpoint/fingerprint below.

## Device CLI

The CLI works by name in DevLink SSH sessions. From another root shell, use `/data/adb/devlink/devlink` or add `/data/adb/devlink` to that shell's PATH.

```sh
devlink on           # stays enabled across reboot until disabled
devlink on 2h        # keeps original wall-clock deadline across reboot
devlink off          # no tunnel/keepalive/DevLink Dropbear process remains
devlink status
devlink status --json
devlink reload      # apply staged update/restart this module only; no reboot
devlink adb on
devlink adb off
devlink lanes 4      # independent WebSocket/TCP lanes, no expiry
devlink lanes 4 15m  # optional expiry only when explicitly requested
devlink lanes 1       # restore the battery-friendly baseline

# Hostname changes retain device credentials and allocated ports.
devlink endpoint arm.jadenjsj.com
devlink endpoint https://cfarm.jadenjsj.com/tools/android
# Direct-IP HTTPS: verify/SNI with the certificate hostname, including Host routing.
devlink endpoint https://138.2.105.41/devlink --tls-name arm.jadenjsj.com
# Optional, deliberately unsafe (enrollment credentials cross the wire in cleartext):
devlink endpoint http://192.168.1.2:18790/custom/path --allow-http
```

An endpoint contains the **base path**, not `/enroll` or `/tunnel`. Arbitrary nested subpaths and custom ports work. All alternate origins must reach the **same server registry and server key**. For a separate server, build for its fingerprint, `devlink off`, then `devlink reset-enrollment` before reconnecting. Revoked identities cannot silently re-enroll themselves.

## Hot-loaded root / live updates

This module has no system mounts, metamodule, kernel/boot patch or reboot dependency. Install/update schedules a detached activation worker that waits until KernelSU finishes its installer, promotes **only DevLink's** staging directory, and starts its normal service. It never replays global ksud stages or restarts Box. Hot-root `late-load` service events work normally; the module service starts after Android boot completion.

For manual reactivation or pending updates, use `devlink reload` or the WebUI **Hot reload** button. Enable/disable, endpoint changes, lane changes and ADB mode already apply live. Updates preserve identity, allocated ports, on/off state and session deadline. Hot-reloading an enabled tunnel briefly disconnects its SSH connections; the detached worker completes the restart even when invoked through that SSH connection. A disabled module stays disabled. Reboot is neither required nor automatically performed.

## Box and Android coexistence

* **SSH:** DevLink owns its own public-key-only Dropbear on `127.0.0.1:18022`. Box keeps its SSH on `127.0.0.1:8022`; DevLink never changes Box configuration, keys, processes or services. Both can run independently. Host SSH public keys are provisioned automatically during enrollment. No password/root-password authentication.
* **ADB:** When Box already has `adbd` listening at `127.0.0.1:5555`, DevLink **borrows** it without restarting or claiming it. Turning DevLink ADB off removes only the reverse forward; Box's listener remains.
* Without that listener, DevLink adds its own `tcp:localhost:15556`, protects it with IPv4/IPv6 non-loopback firewall rules, preserves existing listen specs, and restores its prior configuration on shutdown. If another owner changed the property afterward, DevLink does not overwrite that owner's setting. Existing Box/Vendor services remain their owner's responsibility.
* Android Wireless Debugging's dynamic TLS listener is independent. DevLink does not change `persist.*`, `service.adb.tcp.port`, or TLS-enable properties. Starting/stopping an **owned** ADB listener restarts adbd, briefly disconnecting ADB sessions and possibly changing the wireless port. Borrowing Box's listener does not restart it.
* Normal ADB authentication remains on. If configured on the server, its **ADB public key** is added to Android's authorized host list (private key never sent). Authorization persists after ADB off; no unrelated keys are removed.
* If Box explicitly stops a borrowed ADB listener during a session, toggle DevLink ADB off/on to establish its own listener. DevLink never force-restarts Box.

For file transfer, use `scp -O` (bundled SCP; SFTP is not built in). SSH local forwarding is enabled; remote forwarding and agent/X11 forwarding are disabled.

## Parallel resumable transfers

Use `tools/devlink-transfer` on the origin (Python 3 + OpenSSH only; no Android Python, rsync or SFTP required):

```sh
# The helper automatically enables up to four independent Chisel/TCP lanes.
tools/devlink-transfer push ./large.apk /sdcard/Download/large.apk --port 18622
tools/devlink-transfer pull /sdcard/Movies/capture.mp4 ./capture.mp4 --port 18622
# Adjust concurrency/chunk size; --overwrite is explicit.
tools/devlink-transfer push ./archive.zip /data/local/tmp/archive.zip \
  --port 18622 --jobs 4 --chunk-size 8388608 --overwrite
# Works through the old Box SSH/Tailscale path too, but without extra lanes:
tools/devlink-transfer push ./file.bin /sdcard/file.bin \
  --host mihomo-6sp-arm --port 8022 --tunnels 1
```

**Separate transport lanes, one SSH port:** each lane is a distinct Chisel client/WebSocket/TCP connection to the same HTTPS subpath. The origin rotates new SSH connections across these lanes behind **one stable SSH port per device**; internal listener ports are implementation details, not addresses agents need to juggle. Workers disable SSH connection sharing so separate TCP connections really reach separate lanes, rather than multiplexing everything inside one WebSocket. Extra lanes share the same on-device Dropbear process. Enabling/shrinking lanes does **not** restart the primary tunnel or SSH server. `--tunnels 1` keeps a single connection; `--tunnels 4` is the transfer-helper default.

There is **no default lane time limit**, including the WebUI four-lane toggle. The helper restores the previous lane count after completion or interruption. If the host crashes before cleanup, extra lanes remain enabled until you run `devlink lanes 1` or turn the tunnel off. If desired, explicitly use `--lane-lease 15m` for a renewable crash-cleanup expiry. The session timer (`devlink on 2h`) is independent and still shuts off the whole tunnel at its deadline. There is still a shared physical network/Cloudflare path, so four lanes cannot guarantee a 4× speedup or bypass Android Doze/network suspension.

Transport failures pause and retry automatically (backoff up to 60s). Ctrl-C pauses; **re-run the identical command** to resume verified chunks. State lives in `~/.cache/devlink-transfer`, with upload chunks under `/data/local/tmp/.devlink-transfer` on Android. SHA-256 verifies chunks/final content, and the final destination is committed only after complete verification. Existing different files require `--overwrite`. First host connection uses OpenSSH `accept-new` (TOFU); changed host keys are rejected. Source files must remain unchanged during transfer.

This version transfers **one regular file per invocation**, not a directory tree. Chunk assembly needs roughly **2× file size** free space on the receiving side. Progress persists on failure; complete transfers remove chunks unless `--keep-chunks`. SSH authentication/remote filesystem errors fail clearly rather than retrying forever. Run multiple invocations for independent files, but avoid simultaneous lane ownership changes on the same device; use `--tunnels 1` on secondary invocations.

## Device discovery for AI agents

Run on the origin server (no root or registry-file access required):

```sh
devlink-devices --connected         # labels and ready-to-use SSH/ADB commands
devlink-devices --connected --json  # machine-readable records
curl -fsS 'http://127.0.0.1:18792/devices?connected=1'
```

Records include device ID/model label, `connected`, `active_lanes`, `ssh_host`, `ssh_port`, `ssh_user`, `adb_host`, `adb_port`, and `adb_forwarding`; never enrollment credentials or keys. Omit `--connected` to include offline/revoked enrollments. The API is **loopback-only and not exposed by Caddy/Cloudflare**; remote agents should SSH into the origin or forward that local port. Do not make a public Caddy route to it.

Discovery inspects Linux reverse-listener state without waking devices with SSH probes. A disconnected network may remain marked connected until Chisel detects the dead connection; agents should still retry SSH. `adb_forwarding` means the reverse listener exists, not proof that Android authorized the host or adbd is healthy. Device labels are untrusted descriptive metadata, not ownership verification.

## Origin deployment

The origin listens on **loopback only**: HTTP gateway `127.0.0.1:18790`, Chisel `127.0.0.1:18791`, agent discovery `127.0.0.1:18792`, and each device's allocated SSH/ADB and private backend ports. No tablet SSH/ADB listener is exposed to the Internet. The public surface is HTTPS `/BASE/enroll`, `/BASE/tunnel`, and `/BASE/health` only; the device registry has no public list/admin API.

```sh
# Build locally, or supply SERVER_BINARY=... from a GitHub release.
sudo env SSH_KEYS="$HOME/.ssh/id_ed25519.pub" \
  ADB_KEY="$HOME/.android/adbkey.pub" PREFIX=/devlink \
  ./deploy/install-server.sh
```

Add `deploy/Caddyfile.fragment` **inside the chosen hostname**, before its catch-all. Preserve the path (do not use `handle_path` stripping). For a different path:

1. Set the service `ExecStart` to `devlink-server --path /tools/android` (via a systemd override, or installer `PREFIX`).
2. Change the Caddy matcher to `/tools/android/*`; validate and reload Caddy.
3. On devices: `devlink endpoint https://YOUR_HOST/tools/android`.

The server exposes **one configured prefix at a time**. When migrating without downtime, temporarily rewrite the old public prefix to the new one in Caddy. Client endpoint changes preserve enrollment; you can issue them over the existing SSH session before moving server routes. Switching an active client restarts DevLink SSH/tunnel, so that SSH connection will disconnect.

For Cloudflare, keep WebSockets enabled, bypass cache/challenges/interactive Access for this subpath, and use HTTPS. Do not put an additional interactive login in front of enrollment/tunnel. The Chisel fingerprint authenticates the inner encrypted tunnel despite CDN TLS termination. Enrollment uses verified HTTPS; Cloudflare is trusted for that bootstrap.

```sh
sudo /usr/local/lib/devlink/devlink-server devices
sudo /usr/local/lib/devlink/devlink-server revoke DEVICE_ID
sudo systemctl restart devlink-server  # applies revocation, disconnects all current tunnels
sudo journalctl -u devlink-server
```

The server deliberately retains a deny-all sentinel user: upstream Chisel's empty user list otherwise means **anonymous full access**. Every enrolled device is restricted to **its exact, anchored `R:127.0.0.1:PORT` addresses** (four private SSH lane backends + one ADB port by default; the public loopback SSH frontend is not granted); normal forwards, SOCKS, wildcard binds, other devices' ports, and cfarm target dialing are not authorized.

### Convenience/security tradeoff

Enrollment is intentionally **public and unattended**. Each installation generates a 256-bit random secret locally (not Android IDs or a shared key baked into the ZIP). Repeat enrollment requires that same secret; no manual entry. New enrollment is globally rate limited (one per five seconds) and capped (64 records by default). Registry/keys are private to the service; systemd restricts the origin's privileges, memory and task count.

**This still puts some server resources at risk:** anyone who knows the URL can fill enrollment capacity or use their own permitted listeners. Loopback users/services on the origin can reach connected devices. This is not an Internet-scale untrusted multi-tenant service. Do not treat device labels as proof of ownership. Use revocation/monitoring; change the server exposure model if these tradeoffs stop being acceptable. Never publish `/var/lib/devlink` or `/data/adb/devlink`.

## Battery and failure behavior

Off means no DevLink background process, socket, or periodic wakeup. An enabled session runs one Go process and its idle Dropbear. Normally it contains one Chisel connection; transfer lanes add independent connections only on demand. Chisel keeps its standard 25s keepalive, detects dead pings, and retries indefinitely with exponential backoff up to five minutes; no fast shell watchdog loop. Offline enrollment retries with the same bounded backoff. A small in-process timer checks expiry/module disable even while offline. `off` cancels pending requests/backoff immediately and cleans up resources. Manager disable/removal flags are noticed within five seconds.

Extra lanes consume memory, idle keepalive traffic and reconnect handshakes; during transfers they also compete for bandwidth with other apps. They do not change Box, ADB or Android routing. Multiple devices can be enabled simultaneously, each with an isolated lane pool and its own stable SSH/ADB ports.

No wake lock is acquired. This favors battery over a promise of uninterrupted access during deep Android Doze; network suspension can delay reconnection until Android permits network access again. This is not a measured battery benchmark.

## Builds and tests

```sh
go test -race ./...
go vet ./...
python3 tests/adb_lifecycle.py
python3 tests/transfer.py
python3 tests/hotreload.py
npm ci && npm run build
NDK=/path/to/android-ndk build/build.sh
# Single-architecture local build:
ARCHES=arm64 NDK=/path/to/android-ndk build/build.sh
# Override deployment at build time:
ENDPOINT=https://YOUR_HOST/custom FINGERPRINT='BASE64_SHA256=' \
  NDK=/path/to/android-ndk build/build.sh
```

`deploy/public.json` contains only public endpoint/fingerprint defaults. Build outputs are ignored, secrets are never part of the repository. Android clients use **Android/Bionic cgo DNS**, not Linux's `/etc/resolv.conf` fallback. Android CA stores are explicitly loaded; certificate validation is not disabled. Dropbear is checksum-pinned, patched for Android's root home permissions (same patch as the tested Box build), and compiled without password authentication. Chisel is pinned via `go.mod`/`go.sum` to v1.12.0.

GitHub Actions on Blacksmith runners builds all four Android architectures, runs lifecycle/auth/real-tunnel tests, publishes the module and amd64/arm64 origin binaries, and updates a moving `latest` prerelease. Module WebUI bundles the official `kernelsu` API locally (no external assets).

Primary references: [Chisel](https://github.com/jpillora/chisel), [KernelSU module guide](https://kernelsu.org/guide/module.html), [KernelSU WebUI](https://kernelsu.org/guide/module-webui.html). Box's local tested implementation informed the Dropbear patch and ADB property/firewall handling.
