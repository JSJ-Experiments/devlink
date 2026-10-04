# To do — do not interrupt active development sessions

- Consolidate host/agent commands under `devlink` (e.g. `devlink devices`, `devlink transfer`). Keep the origin daemon named `devlink-server`.
- Simplify GitHub release assets: one obvious Android module ZIP and clearly labeled origin bundles; remove duplicate versioned ZIPs and standalone helper clutter.
- Publish/apply these packaging changes only during an agreed maintenance window.
- Publish the tested cgroup-aware launcher in the next planned module build; the current device has the equivalent live fix already, and must not be restarted just to publish it.
