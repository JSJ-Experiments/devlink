# DevLink 0.1

- Generic KernelSU module with unattended per-install enrollment and unique origin ports.
- Separate loopback-only Dropbear; optional Box-aware ADB forwarding.
- Persistent enable/disable, timed sessions, KernelSU Action and bundled WebUI.
- Configurable hostname/IP/TLS-name/port and nested base path.
- Restricted reverse-only authorization; origin resource limits, capacity cap and revocation.
- Verified TLS, pinned Chisel identity, Android-native DNS and automatic reconnect.
- Automated four-architecture Android module and two-architecture origin builds.

- On-demand independent WebSocket/TCP lanes and a parallel checksum-verified transfer helper with disconnect retry, pause/resume and lane leases.
- Immediate hot-root installation/updates and WebUI/CLI hot reload without reboot or replaying other modules.
- Loopback-only agent discovery API and `devlink devices --connected --json`.
- One stable SSH port per device pools independent WebSocket lanes; no extra public lane ports.
- No default lane expiry in WebUI/transfers; optional renewable leases remain available.
- GitHub Actions builds run on Blacksmith.
- Explicit unlimited Chisel library retries on all lanes, with disconnect/reconnect regression tests.
- Fixed two-second retries (no exponential backoff) for every tunnel lane, enrollment and transfers.
- Root launcher escapes Android app/freezer process groups so closing KernelSU Manager cannot freeze its inherited DevLink processes.
- Unified origin CLI: `devlink devices`, `info`, `ssh`, `adb`, and `transfer --device`; daemon/admin binary remains `devlink-server`. Legacy helper names remain compatibility wrappers.
- Simplified release: one Android ZIP and two self-contained Linux origin bundles; stale versioned ZIPs and standalone helper assets are removed.
- Keep Dropbear's parent OS thread alive, preventing Linux thread-bound parent-death signals from killing it prematurely; recover unexpected SSH server exits without stopping Chisel lanes.
- Renew explicitly requested short transfer-lane leases before their deadline.
