# Maintenance checklist

- [x] Consolidate host/agent commands under `devlink`; daemon/admin remains `devlink-server`.
- [x] Simplify release downloads to one Android ZIP and two clearly labeled origin bundles.
- [x] Package the tested Android cgroup-aware launcher for hot-root installations.
- [ ] Publish during the user-authorized maintenance window, then verify the release assets.
- [ ] Hot-install on the real device; verify stable ports/state, root cgroups, SSH, transfer resume and reconnect, with no reboot or Box/adbd restart.
