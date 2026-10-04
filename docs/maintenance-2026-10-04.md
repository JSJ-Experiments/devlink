# Maintenance validation — 2026-10-04

Release/runtime: **0.1.09e07d3**, built on Blacksmith and hot-installed on a rooted arm64 Android device (`24018RPACC`). The public Android ZIP remains generic and contains all four supported architectures; an arm64-only subset was used privately to reduce the maintenance upload. Other Android architectures and the amd64 origin bundle were build-tested, not physically retested here.

## Reliability

- Before upgrading, the live cgroup fix kept the same daemon/Dropbear processes running for nearly six hours, including automatic recovery from network disconnects. This observation applies to the earlier runtime with the equivalent live launcher fix, not a six-hour soak of the final release.
- The packaged launcher was tested against real Android cgroups: a probe deliberately entered a temporary child cgroup, then the unchanged launcher moved itself back to the root cgroup before native execution. The temporary group/probe files were removed; no manager/Box group or global freezer setting was changed.
- Hot installs promoted only DevLink without reboot. The final daemon and Dropbear both run in the unified root cgroup, not KernelSU Manager's app cgroup.
- A deliberate Dropbear termination recovered after the fixed two-second delay. The final daemon PID stayed unchanged; the SSH server PID changed, SSH worked again, and status immediately showed connected without a stale recovery error. Independent Chisel lanes remained active.
- Linux's thread-bound parent-death signal is handled by retaining Dropbear's launching OS thread for its lifetime. Unit tests cover normal lifetime/reaping and failed starts.

## Transfers and discovery

- Device-selected upload/download of a 1,048,593-byte file produced matching SHA-256 hashes.
- An 8,388,608-byte download was interrupted with Ctrl-C after four verified chunks. Re-running the identical command reused those chunks. Restarting the origin during resume caused transport failures, fixed two-second retries and eventual checksum-correct completion.
- A new device-selected transfer was started while the origin/discovery service was stopped. It retried connection-refused discovery failures every two seconds, then retried SSH while the origin/device reconnected, and completed with the correct checksum. Registered offline devices are valid queued/resumable targets; invalid/ambiguous/revoked selection and permanent HTTP errors fail clearly.
- Discovery stays loopback-only. The public `/devlink/devices` URL returned HTTP 404. Both configured public hostnames' `/devlink/health` endpoints passed.

## ADB and coexistence

- Normal ADB forwarding borrowed Box's existing localhost:5555 listener. `devlink adb DEVICE -- shell ...` returned the expected model and an authenticated Android shell.
- The original adbd PID, Box SSH PID and ADB listen property remained unchanged throughout the ADB checks.
- A temporary obstruction of DevLink's own ADB-public-key output path tested setup failure before any ADB listener mutation. Status reported an ADB warning while SSH remained running and connected. The public-key file was restored and ADB returned to its original off setting. Unit tests additionally cover failed helpers, malformed/out-of-range ports and rollback.
- The boot ID did not change. Server identity, registry and operator public-key files remained byte-identical, as did `/etc/caddy/Caddyfile`.

## Packaging and final state

- Release assets: `devlink.zip`, `devlink-origin-linux-amd64.tar.gz`, `devlink-origin-linux-arm64.tar.gz`, `checksums.txt`, `update.json`. Old versioned ZIPs and standalone helpers were pruned only after successful uploads.
- The prebuilt arm64 origin bundle's installer was exercised on the origin; `devlink` and `devlink-server` are installed. Existing helper names remain compatibility wrappers, not separate release downloads.
- CI passed Go race tests/vet, Python transfer/discovery/lifecycle/packaging tests, shell syntax checks, WebUI build and four-architecture Android builds.
- Final tested device state: enabled indefinitely, four lanes without expiry, ADB off, original enrollment, default cfarm subpath, SSH **18622**, optional ADB **18623**. Module listing reports enabled, Action and WebUI present, no pending reboot/update.

This is a maintenance stress test, not a battery benchmark or a guarantee against deep Doze, root loss, OS/daemon termination or all network outages. Existing SSH sessions cannot survive every tunnel/server restart; SSH commands are deliberately **not** replayed automatically. Verified chunk transfers are resumable and retryable.
