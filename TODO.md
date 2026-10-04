# Maintenance checklist

- [x] Consolidate host/agent commands under `devlink`; daemon/admin remains `devlink-server`.
- [x] Simplify release downloads to one Android ZIP and two clearly labeled origin bundles.
- [x] Package and physically validate the Android cgroup-aware launcher for hot-root installations.
- [x] Harden Dropbear parent-thread lifetime and recover unexpected SSH server exits without stopping tunnel lanes.
- [x] Refresh initial/recovered status promptly and preserve SSH when optional ADB setup fails.
- [x] Retry temporary discovery failures and registered-offline-device transfers without exponential backoff.
- [x] Publish during the user-authorized maintenance window and verify release assets/checksums.
- [x] Hot-install on the real device; verify stable ports/state, root cgroups, SSH/ADB coexistence, transfer pause/resume and reconnect, without reboot or Box/adbd restart.

Evidence and scope: [maintenance validation](docs/maintenance-2026-10-04.md). Operational limitations remain documented in README; no outstanding items from this maintenance checklist.
