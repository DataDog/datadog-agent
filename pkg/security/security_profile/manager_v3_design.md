# Security Profiles v3 (experimental): event-sourced profiles

Status: proof of concept. Code lives in `manager_v3.go` and `manager_v3_snapshot.go`,
selected by `runtime_security_config.security_profile.v3.enabled`.

## Goal

v3 keeps the detection behavior of v2 but moves profile construction off the agent.
The agent never builds an activity tree and never sends a profile. It only decides,
per workload, whether a given behavior has been seen before, and if it is new it sends
one `anomaly_detection` event describing it. The backend consumes that event stream and
rebuilds the profiles, keyed by host plus image tag.

Why: the activity tree is the expensive part on the agent (memory, per-workload trees,
eviction, transcoding). v3 replaces it with a per-workload set of fixed-size hashes. The
behavior the backend sees is the same set of nodes v2 would have created; only the place
where the tree is assembled changes.

## What "same behavior as v2" means

v3 deliberately reuses v2's node identity rules so the backend rebuilds the same nodes v2
would have. For every event, v3 computes the set of node identities that event would touch
in v2's activity tree, and emits only if at least one of them is new. This mirrors v2, which
inserts an event and emits when the insert adds a new node.

Node identity per event type (matches `activity_tree` insert keys):

- Process node: the process lineage, walked with `GetNextAncestorBinaryOrArgv0` (busybox
  aware, skips same-binary ancestors). Each ancestor contributes its reduced binary path,
  its argv0 only when the binary is busybox, and the full argv when `differentiate_args` is
  on. This single item is type-agnostic, so `exec`, `connect`, and any event type v2 does
  not key further yield only process-node novelty, exactly like v2.
- File open: the reduced file path. Flags and mode are NOT part of identity (see below).
- DNS: question name plus type.
- Bind: family, IP, port, protocol.
- IMDS: the whole `IMDS` struct.
- Syscalls: one item per syscall number (set semantics).
- Capabilities: one item per attempted capability plus its "capable" bool (set semantics).
- Network flow: one item per flow, device plus five-tuple (set semantics).
- Exit: one item carrying the exit, with no process-node item, so a process whose only event
  is an exit is not reported. Matches v2, which never emits on exit but does stamp exit time.

Paths are canonicalized with the same rule-based `PathsReducer` v2 uses (pids, container ids,
block devices, service-account tokens, and so on). v3 does NOT replicate v2's learned path
generalization (`PathPatternMatch`), which is stateful and fuzzy. That generalization moves
to the backend. See "What the backend must do".

## Hashing

Each node identity is hashed with SHA-256 truncated to 128 bits, stored as raw bytes
(`[16]byte`), not hex.

The threat is second-preimage, not collision: an attacker who already ran some behavior
wants to craft a different real behavior whose hash equals one already marked "seen", so the
new behavior is treated as known and never reported. That requires finding a second preimage
of a specific hash, which is 2^128 here. A fast non-cryptographic hash (FNV and similar) is
cheaply forgeable and was rejected for this reason.

A domain byte separates the process-node item (prefix only) from leaf items, so a process
item and a leaf can never collide. Each leaf also writes its own event-type tag.

## Mutable node state

Three fields change as a node recurs: `last_seen`, and for file opens the accumulated open
`flags` and `mode`. v3 stores these as values in the seen map, keyed by the node identity hash:

```
seen: map[WorkloadSelector]map[[16]byte]*nodeState   // nodeState{lastSeen, flags, mode}
```

On every event, `record`:

- creates the state and marks the event for emission if the identity is new,
- for a file open, ORs in new flag/mode bits and marks the event for emission if any bit is
  new (matching v2's `enrichFromEvent`, which ORs flags/mode on every open),
- otherwise advances `last_seen` in place and stages the node for a piggybacked refresh.

There is no periodic reset. Identities are stable keys; only the changeable fields move.

## last_seen delivery: piggybacking

A node that recurs with nothing new advances `last_seen` locally but does not itself trigger
a send. Its refresh rides on the next emitted event for the same workload. Each emitted
`anomaly_detection` event carries:

- `security_profile_node_hashes`: the identity hashes of the nodes this event represents, so
  the backend can index node to hash.
- `security_profile_refreshes`: a list of `{hash, last_seen}` for other nodes of the same
  workload that recurred unchanged since the last emission.

This avoids a separate wire channel and a timer. Refreshes only leave when something else is
already being sent, so there is no re-emit burst. The tradeoff is timeliness: a workload that
is active but produces nothing new stalls its `last_seen` updates until the next real
emission. That is acceptable, `last_seen` freshness only matters for anomaly detection
staleness, which is out of scope for the PoC.

These fields are spliced into the serialized event by a wrapper marshaler in the probe
(`anomalyDetectionV3Marshaler`), so the easyjson event schema and the committed JSON schema
are untouched. The backend ignores unknown fields until it is ready to consume them.

## Per-tag keying

The seen set is keyed by `(image_name, image_tag)`, not by image alone as v2 keys profiles.
Each tag re-emits its full behavior set, so the backend can rebuild a complete profile per
host plus image tag. A new tag (new deploy) starts a fresh seen set and re-emits.

## Tag-resolution buffering

Workload tags are not always resolved when the first events for a cgroup arrive. v3 buffers
events per cgroup until tags resolve, then drains them in order. Buffered events are deep
copied, dropped if the cgroup waits more than 10s for the first flush or 60s overall. This is
the same buffering shape v2 uses. v3 never drops events silently while tags are pending; it
holds them.

## Snapshot phase

Behavior that predates tracing would otherwise be missed. At startup `snapshotExistingProcesses`
walks the process cache (`ProcessResolver.Walk`, which the resolver already populated from a
`/proc` snapshot), and for each already-running containerized process whose tags resolve, reads
its current open files and bound sockets, synthesizes `open` and `bind` events, and routes them
through the normal emit path marked snapshot-origin. On the wire these carry
`security_profile_generation: "snapshot"` so the backend tags those nodes with the `Snapshot`
generation type.

Processes that start after the one-time walk are not snapshotted; they are seen live over ebpf.
This is the same split the dump pipeline uses: snapshot what already exists, rely on ebpf for
what starts later.

PoC limitations:

- Only open files and bound sockets. mmap'd files are not snapshotted yet (v2 does snapshot them).
- Snapshot opens carry no flags/mode (the kernel does not expose original open flags via `/proc`;
  v2 has the same limitation). Those nodes start at flags/mode 0 and fill in on the next live open.
- The snapshot runs in a goroutine at `Start`, so it races live events. The hash set makes this
  safe for dedup; the only effect is that a node's generation label is set by whichever path,
  snapshot or live, emits it first.

## Generation type

`GenerationType` is a per-node byte in v2 (`Unknown`, `Runtime`, `Snapshot`, `ProfileDrift`,
`WorkloadWarmup`) recording how a node first entered the tree. v2 security profiles only ever
use `Runtime`; the `Snapshot` generation comes from the activity-dump pipeline. v3 carries only
the snapshot-vs-runtime distinction on the wire (`security_profile_generation`), which is enough
for the backend to tag snapshot-origin nodes.

## Wire contract

Each emitted `anomaly_detection` event is a full `model.Event` plus these sibling fields:

- `security_profile_node_hashes`: `[][]byte` (16-byte identities, base64 in JSON).
- `security_profile_refreshes`: `[{ "hash": bytes, "last_seen": int64 }]`.
- `security_profile_generation`: `"snapshot"` when snapshot-origin, omitted otherwise.

The full event already carries first-seen (its timestamp), the process lineage, open flags/mode,
and all type-specific fields. The extra fields exist only for node-to-hash indexing, last_seen
refreshes, and generation tagging.

## Can the backend rebuild a v2 profile?

Yes for structure and mutable state, with known exceptions.

Reconstructable from the stream:

- Node set and identity, from each novel full event.
- Tree edges (parent/child), from the ancestor lineage in each event.
- Per-tag first-seen, from the first novel event per (image, tag).
- Per-tag last-seen, from piggybacked refreshes.
- Open flags/mode union, from full events re-emitted on new flag/mode bits.
- Syscalls, capabilities, IMDS, network devices, DNS, sockets.
- Per-tag membership, from the per-tag keying.

Not reconstructable from the agent stream:

1. Learned path wildcards. v2 collapses many concrete paths into one wildcarded node via
   `PathPatternMatch`. v3 sends concrete paths and relies on the backend to learn the wildcards.
   Until the backend does, v3 profiles hold concrete nodes where v2 had generalized ones. This is
   the fundamental, accepted difference.
2. Full generation-type nuance. v3 distinguishes only snapshot vs runtime. v2's dump pipeline has
   more (`ProfileDrift`, `WorkloadWarmup`), which v3 has no equivalent for.
3. MatchedRules. Per node, v2 records the detection rules that fired on events touching the node.
   v3 does not carry this, by decision. At v3 emit time (inside `ProcessEvent`, before the rule
   engine runs for that event) `event.Rules` is not populated, and the annotation is not part of
   behavioral identity. The backend can recover it by joining the anomaly-event stream with the
   rule-match signal stream server-side.

## What the backend must do

- Fold the `anomaly_detection` stream by `(host, image_name, image_tag)` into a profile.
- Build a node-to-hash index from `security_profile_node_hashes`, and advance `last_seen` from
  `security_profile_refreshes`.
- OR open flags/mode across events for the same file node.
- Learn path wildcards (the generalization v2 did on the agent).
- Tag snapshot-origin nodes from `security_profile_generation`.
- Optionally join rule-match signals to recover per-node MatchedRules.

## Open items on the agent

1. Memory lifecycle. `seen` and `pendingRefresh` are keyed by workload selector and are never
   freed. `onCGroupDeleted` cleans the pending-event buffers but not these. A long-running agent
   leaks one hash set per (image, tag) ever seen, growing on every deploy. Needs a cgroup-to-selector
   eviction or a TTL/LRU on the seen sets.
2. Observability. `SendStats` is a no-op. There are no counters for emitted events, seen-set size,
   or snapshot coverage. Only the tag-resolution drop metric is wired.

Neither changes the wire contract, so the backend can be built against the contract above while
these are addressed.

## Design decisions, for the record

- Standalone, not embedded. v3 does not embed v2. It copies the small amount of plumbing it needs
  (tag buffering, cgroup tracking) so it can be changed freely during the PoC.
- Exact v2 parity is impossible statelessly, because `PathPatternMatch` is fuzzy and stateful.
  Deterministic rule-based canonicalization is replicable; learned wildcarding is not, and moves
  to the backend.
- SHA-256/128 over a fast hash, because the threat is second-preimage and a forgeable hash would
  let an attacker mark crafted behavior as already-seen.
- Piggyback over a dedicated edit channel, because it reuses the existing send path, needs no timer,
  and self-rate-limits. Timeliness of last_seen is not important for the PoC.
- MatchedRules left to the backend, because it is a server-side join and is not available at agent
  emit time anyway.
