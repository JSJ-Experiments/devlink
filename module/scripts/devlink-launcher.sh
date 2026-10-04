#!/system/bin/sh
# Root-only, per-process escape from Android's cached app freezer/task group.
# Never disable Android freezing globally or change the manager/Box group.
for group in "/sys/fs/cgroup/cgroup.procs" "/sys/fs/cgroup/freezer/cgroup.procs" "/dev/freezer/cgroup.procs"; do
 if [ -w "$group" ]; then printf '%s\n' "$$" > "$group" 2>/dev/null || true; fi
done
exec "${0}.native" "$@"
