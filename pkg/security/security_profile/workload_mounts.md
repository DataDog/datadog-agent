# Workload mounts in CWS security profiles

This documents the mount information CWS attaches to a workload's V2 security
profile, and the flag-encoding contract the backend must rely on. It is the
source of truth for how `mount_flags` is normalized; the backend consumes these
values and must interpret them exactly as described here.

## What is captured

For each container workload, the V2 profile carries a single per-workload mount
table: a flat, deduplicated union of every mount observed across all of the
workload's mount namespaces. Each mount records:

- **mount point** — path of the mount
- **mount root** — root within the source filesystem
- **filesystem** — filesystem type (e.g. `overlay`, `tmpfs`, `ext4`)
- **mount flags** — a normalized per-mount attribute bitmask (see below)
- **base namespace** — whether the mount belongs to the base (first-seen) mount
  namespace of the workload (see below)

In the payload (`agent-payload`, `proto/cws/dumpsv1/activity_dump.proto`):

```proto
message MountNode {
    NodeBase node_base = 1;    // per-image-tag first/last seen
    string mount_point = 2;
    string mount_root  = 3;
    string filesystem  = 4;
    uint32 mount_flags = 5;    // normalized per-mount attributes (see below)
    bool base_namespace = 7;   // observed in the workload's base mount namespace
}
```

`repeated MountNode mounts` is attached at the tree level of both `SecDump` and
`SecurityProfile`.

## Purpose (workload hardening)

The backend uses this to reason about a container's filesystem posture:

- A root filesystem that is writable and executable but is never written to
  should be recommended read-only.
- Adjacent (non-root) mounts may be writable but should not be executable.

Determining "writable" / "executable" requires the per-mount flags below,
combined with file-write activity already tracked as file nodes in the profile.

## Table semantics

- **Append-only.** Mounts are never removed from a profile, including on
  `umount`. The table is the union of everything ever observed for the workload.
- **Flat across namespaces.** The mount-namespace inode is not a key: the same
  mount seen in different namespaces (e.g. across container restarts, which each
  get a fresh, ephemeral namespace inode) collapses into one entry. This keeps
  the table bounded by the number of distinct mounts rather than the number of
  runs, and keeps the meaningless per-run inode out of the payload.
- **Dedup key** = `(mount_point, mount_root, filesystem, mount_flags)`. Re-seeing
  the same mount updates `node_base` last-seen only.
- **Flag differences create a new entry.** If a mount reappears with different
  `mount_flags`, both entries are kept as history (e.g. a remount that flips
  `ro`/`rw`).

## Flag normalization contract (IMPORTANT for the backend)

`mount_flags` is **not** a raw kernel value. Per-mount flags reach the agent in
two different encodings depending on the source:

- live eBPF mount events expose `vfsmount.mnt_flags` (`MNT_*`)
- the `statmount(2)` / procfs startup snapshot exposes `MOUNT_ATTR_*` /
  mountinfo option strings

The same mount can be observed by either source over a profile's lifetime (for
example, a container already running when system-probe (re)starts is captured by
the snapshot, while its later changes come from eBPF). Because the dedup key
includes the flags, storing each source's raw value verbatim would make the same
mount key differently by source and duplicate. To avoid this, the agent
normalizes every source into a single canonical bitmask.

The canonical layout adopts the stable UAPI `MOUNT_ATTR_*` values. Only
security-relevant bits that are losslessly derivable from every source are kept;
atime-related bits are intentionally excluded (noise for hardening and their
encoding diverges across sources).

| Canonical bit (`mount_flags`) | value      | eBPF `mnt_flags`        | procfs option  |
| ----------------------------- | ---------- | ----------------------- | -------------- |
| RDONLY                        | `0x1`      | `MNT_READONLY` (`0x40`) | `ro`           |
| NOSUID                        | `0x2`      | `MNT_NOSUID` (`0x01`)   | `nosuid`       |
| NODEV                         | `0x4`      | `MNT_NODEV` (`0x02`)    | `nodev`        |
| NOEXEC                        | `0x8`      | `MNT_NOEXEC` (`0x04`)   | `noexec`       |
| NOSYMFOLLOW                   | `0x200000` | `MNT_NOSYMFOLLOW`(`0x80`)| `nosymfollow` |

Backend interpretation:

- **writable** = `RDONLY` bit is not set
- **executable** = `NOEXEC` bit is not set

## Base namespace

Mounts are collapsed into one flat table, but each entry carries a
`base_namespace` flag. The **base namespace** is the first mount namespace
observed for the workload (in practice, the namespace snapshotted when the
profile is first seeded). A mount has `base_namespace = true` if it was ever
observed in that base namespace, and `false` if it was only ever seen in a later
namespace (for example, a mount that appears only after a container restart or
in a differently-configured instance of the same image).

Backend interpretation: `base_namespace = true` marks the workload's canonical
mount topology; `false` entries are deviations observed in other instances of
the workload. The raw mount-namespace inode is intentionally not exposed — it is
an ephemeral, per-run value with no cross-run meaning.

A process can `setns` into another mount namespace; those mounts (if observed)
simply fold into the same flat table under the dedup rules above.
