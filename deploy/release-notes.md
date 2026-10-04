## Which download?

- **Rooted Android / KernelSU:** install **devlink.zip**. One ZIP supports arm64, ARM, x86-64 and x86. Hot-root installs activate without reboot; upgrades preserve enrollment and settings.
- **Linux origin/server:** download **devlink-origin-linux-arm64.tar.gz** (ARM64) or **devlink-origin-linux-amd64.tar.gz** (x86-64). Each includes `devlink-server`, the unified `devlink` CLI, installer and Caddy fragment.
- `checksums.txt` verifies downloads. `update.json` is KernelSU update metadata, not another installer.

First Android install auto-enrolls against cfarm and enables SSH for two hours. Use the action/WebUI or `devlink on` for indefinite operation. ADB is optional. SSH listeners remain origin-loopback only. Four independent WebSocket lanes share one stable SSH port per device.

Origin tools: `devlink devices --connected --json`, `devlink ssh DEVICE`, `devlink transfer push FILE DEST --device DEVICE`. Transfers retry at a fixed two-second interval and resume verified chunks after interruption. Android's tunnel also reconnects indefinitely without exponential backoff.

See [README](https://github.com/JSJ-Experiments/devlink#readme) for enrollment security, custom domains/subpaths, battery behavior and origin setup.
