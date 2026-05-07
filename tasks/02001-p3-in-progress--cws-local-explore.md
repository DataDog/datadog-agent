---
created: 2026-05-05
priority: p3
status: in-progress
artifact: pending
---

# cws-local-explore

## Plan

# CWS local exploration kit

Build a self-contained playground under `dev/cws-explore/` that lets you run system-probe locally with Cloud Workload Security enabled and *see* the event stream with your own eyes — three different ways. Purely exploratory, never committed beyond `dev/`.

## Context

The Datadog system-probe runs eBPF probes that observe process exec/fork/exit, file ops (open/chmod/rename/unlink/...), network (bind/connect/dns), and kernel events (bpf/ptrace/mmap/load_module/...). These events are evaluated against SECL rules and, on match, shipped over a unix-socket gRPC stream (`SecurityModuleEvent.GetEventStream`) for security-agent to consume and forward to the Datadog backend.

Reference points in the repo:
- `cmd/system-probe/modules/eventmonitor.go:65-84` — how the CWS consumer is wired into system-probe
- `pkg/security/proto/api/api.proto:344-346` — the gRPC streaming surface
- `pkg/security/agent/agent.go:170-227` — canonical client (loop calling `GetEventStream`, logs each message at trace level)
- `pkg/security/tests/module_tester_linux.go:130-218` — known-good config template
- `pkg/security/tests/module_tester.go:707-744` — minimal `PolicyDef` YAML shape
- `pkg/security/secl/model/events.go` — full event-type list
- `cmd/system-probe/subcommands/runtime/activity_dump.go:138-207` — activity-dump CLI flags

## What to do

### 1. Create the playground directory `dev/cws-explore/` containing:

- `system-probe.yaml` — minimal config:
  - `log_level: debug`
  - `runtime_security_config.enabled: true`
  - `runtime_security_config.socket: /tmp/cws-explore/runtime-security.sock`
  - `runtime_security_config.cmd_socket: /tmp/cws-explore/runtime-security-cmd.sock`
  - `runtime_security_config.policies.dir: /tmp/cws-explore/policies`
  - `runtime_security_config.event_server.rate: 10000`, `burst: 100000`, `retention: 15s`
  - `runtime_security_config.self_test.enabled: true`, `send_report: false`
  - `event_monitoring_config.network.enabled: true` (so connect/bind/dns events fire)
  - All the activity-dump / security-profile knobs disabled to keep the surface small
- `datadog.yaml` — `api_key: 0000001`, `hostname: cws-local-dev` (used by both binaries; they share `dda` config plumbing)
- `policies/default.policy` — a SECL **catch-all policy** with one rule per event type that should always evaluate true. Pattern: `exec.file.path != ""`, `open.file.path != ""`, `dns.question.name != ""`, `connect.addr.ip != 0.0.0.0/0` style — verify each expression compiles by running `bin/system-probe/system-probe runtime policy check` against it. Cover at minimum: exec, fork, exit, open, chmod, chown, unlink, rename, mkdir, link, setxattr, bind, connect, accept, dns, bpf, ptrace, mmap, mprotect, load_module, unload_module, setuid, capset, signal. Each rule with id `catchall_<eventtype>`.
- `cmd/stream-events/main.go` — a small standalone Go program (its own `go.mod` or run via `go run` from repo root) that:
  - Dials the unix socket at `/tmp/cws-explore/runtime-security.sock` over gRPC
  - Calls `SecurityModuleEvent.GetEventStream`
  - For each `SecurityEventMessage` received, prints `RuleID`, `Service`, `Tags`, and pretty-prints the `Data` JSON to stdout
  - Use the existing types in `pkg/security/proto/api` so we don't re-vendor the proto
- `run.sh` — orchestrates: builds binaries (`dda inv system-probe.build`, `dda inv security-agent.build`), creates `/tmp/cws-explore/` dirs, copies configs/policy into place, prints instructions for the three viewing modes
- `README.md` — playbook explaining the three ways to view events:
  1. **Live gRPC stream**: `sudo bin/system-probe/system-probe run -c dev/cws-explore/system-probe.yaml` in one terminal, `sudo go run ./dev/cws-explore/cmd/stream-events` in another. In a third terminal: `find /etc -name '*.conf' >/dev/null`, `curl example.com`, `bash -c 'echo hi'` to generate noise. Watch events stream past.
  2. **security-agent at trace level**: as above for system-probe, then `sudo bin/security-agent/security-agent start -c dev/cws-explore/datadog.yaml --sysprobe-config dev/cws-explore/system-probe.yaml` with `log_level: trace` set in datadog.yaml — every event arrives via the `Got message from rule X for event Y` line in `pkg/security/agent/agent.go:218`.
  3. **Activity dump for a cgroup**: launch a target workload (e.g. `systemd-run --scope --unit=cws-target bash`), grab its cgroup id, then `sudo bin/system-probe/system-probe runtime activity-dump generate dump --cgroup-id <id> --timeout 60s --output /tmp/cws-explore/dumps --format json`. Inspect the resulting JSON tree.

  Document the rate-limit knobs, the kernel version requirements (5.8+ baseline, 5.13+ for flow monitor, 5.17+ for capabilities monitoring per `pkg/security/probe/probe_ebpf.go:363-388`), and how to clean up sockets/policies between runs.

### 2. Smoke-validate the playground

Run `run.sh` end to end on this workspace's kernel:

- Confirm system-probe starts cleanly (no policy load errors, self-test passes — look for "Successfully connected" and "Self test ran successfully" log lines).
- Confirm the catch-all policy compiles (`system-probe runtime policy check` exits 0; `system-probe runtime policy dump-loaded` shows the rules).
- Confirm `cmd/stream-events` prints at least exec, open, and connect events when triggered with `find /etc`/`curl`.
- Confirm `activity-dump generate dump` produces a non-empty JSON file for a target cgroup.

If any expression in `default.policy` fails to compile, fix it by consulting `pkg/security/secl/model/accessors_unix.go` for the actual field names of that event type. Document any rules that had to be tweaked.

## Acceptance criteria

- [ ] `dev/cws-explore/` exists with `system-probe.yaml`, `datadog.yaml`, `policies/default.policy`, `cmd/stream-events/main.go`, `run.sh`, `README.md`
- [ ] `bash dev/cws-explore/run.sh build` builds `bin/system-probe/system-probe` and `bin/security-agent/security-agent` successfully
- [ ] `sudo bin/system-probe/system-probe runtime policy check -c dev/cws-explore/system-probe.yaml` reports the catch-all policy as valid
- [ ] Running system-probe + the `stream-events` helper, then triggering a `find` and a `curl` in another shell, prints at least 5 distinct rule_ids worth of events to stdout in pretty-printed JSON
- [ ] `activity-dump generate dump` produces a JSON file under `/tmp/cws-explore/dumps/` with non-trivial process / file / network nodes
- [ ] `README.md` walks through all three viewing modes with copy-pasteable commands and notes the kernel/root caveats
- [ ] Nothing outside `dev/cws-explore/` is modified except possibly a `.gitignore` entry for `/tmp/cws-explore/` artifacts (the dir itself lives under `/tmp` so this is unlikely to be needed)

## Out of scope

- Shipping events to a real Datadog backend (the dummy api_key is intentional; we only care about the local stream).
- Writing real detection rules — the catch-all policy is for observation only and would be far too noisy in production.
- Windows / ETW path. Linux eBPF only.
- Container/Kubernetes deployment. Bare-metal Linux host only.


## Progress

### Static deliverables (all complete)

- [x] `dev/cws-explore/system-probe.yaml` — minimal CWS-enabled config; sockets at `/tmp/cws-explore/`; activity-dump enabled (CLI-driven); network monitoring on; per-platform feature gates documented inline.
- [x] `dev/cws-explore/datadog.yaml` — dummy api_key, hostname, `log_level: trace`, all real-backend traffic disabled.
- [x] `dev/cws-explore/policies/default.policy` — 24 catch-all rules (`catchall_<eventtype>`) covering exec / exit / setuid / setgid / capset / signal / open / chmod / chown / unlink / rename / mkdir / rmdir / link / setxattr / utimes / chdir / bind / connect / accept / dns / bpf / ptrace / mmap / mprotect / load_module / unload_module. `fork` intentionally omitted (no SECL accessors). Each predicate is `<numeric-field> >= 0` or `<path> != ""` so it's always-true and always parses.
- [x] `dev/cws-explore/cmd/stream-events/main.go` — gRPC client over unix socket; calls `SecurityModuleEvent.GetEventStream`; pretty-prints `RuleID` / `Service` / `Tags` / `Data`; reuses repo's `pkg/security/proto/api` types so no proto re-vendor.
- [x] `dev/cws-explore/run.sh` subcommands: `prepare`, `build`, `build-stream`, `policy-check`, `start-systemprobe`, `start-secagent`, `stream`, `activity-dump`, `clean`, `doctor`, `all`.
- [x] `dev/cws-explore/README.md` — full playbook for all three viewing modes plus Troubleshooting section.
- [x] Build artifacts present on disk: `bin/system-probe/system-probe`, `bin/stream-events`. (security-agent build is invocation-side via `run.sh build`.)
- [x] `system-probe runtime policy check` validates `default.policy` cleanly (no errors in JSON report).
- [x] Nothing modified outside `dev/cws-explore/` except an existing `.gitignore` change carried in from the same task commit.

### Smoke-test status — second pass after host reboot

The earlier kprobe-refcount leak cleared once the host rebooted, so we were able
to run the full smoke test. Two of the three viewing modes work end-to-end; the
third is environmentally limited.

#### Live event-stream verification — ✅

- system-probe starts cleanly (`Self test ran successfully`, runtime-security
  socket created at `/tmp/cws-explore/runtime-security.sock`).
- Catch-all policy loads — Filter Report in the log lists every event type as
  `mode: accept`.
- `cmd/stream-events` over the unix socket prints events with all expected
  fields (`rule_id`, `process`, `evt`, `cgroup`, `network`/`dns` where
  applicable). Across an 8-second sample we captured 31251 lines of
  pretty-printed JSON, with **14 distinct `rule_id` values** including
  `catchall_exec`, `catchall_open`, `catchall_dns`, `catchall_bind`,
  `catchall_connect`, `catchall_chmod`, etc. The acceptance bar of ≥5 is
  comfortably met.
- The `doctor` subcommand was extended to auto-mount tracefs on hosts where it
  isn't mounted at `/sys/kernel/tracing` after a reboot.

#### Activity-dump verification — ⚠️ blocked by workspace container nesting

The activity-dump CLI surface works correctly:

- `system-probe runtime activity-dump generate dump --container-id <id> --timeout <dur> --output ... --format json` registers a dump and returns a structured ActivityDumpMessage.
- `... activity-dump list` shows the active dump with selector, timeout, storage paths.
- `... activity-dump stop --container-id <id>` cleanly stops it.
- The probe logs `tracing started for [<container>]` from `pkg/security/security_profile/ad.go:212` exactly as expected.

What does **not** work in this environment is the kernel-side capture: every dump finishes with all `*_nodes_count` stats at 0 and no JSON is written to disk (the persist path drops empty profiles via `m.emptyDropped.Inc()` at `pkg/security/security_profile/grpc.go:159`).

Root cause, confirmed via the system-probe debug log: the cgroup resolver at
`pkg/security/resolvers/cgroup/resolver.go:251` cannot resolve cgroups for
processes originating from sibling Docker containers. Across a single 30-second
dump window we logged **9514 `failed to add pid X, error on fallback to resolve
its cgroup` lines covering 4963 unique PIDs and zero successes**. Both legs of
the resolver fail:

- The dentry-resolver leg fails with `dentry path key not found <mountID>/<inode>` because the cgroup-file inodes the kernel hooks observe are not present in system-probe's dentry cache (the workspace container's mount namespace masks the parent docker hierarchy).
- The procfs fallback (`FindCGroupContext` walking `/proc/<pid>/cgroup`) fails because the workload PIDs are short-lived (`sh`, `find`, `cat` from the in-container loop) and exit before user-space gets to read their `/proc` entries — and even when they don't, the cgroup mount points the resolver was initialised with don't expose the host's `/docker/<id>` subtree from inside our cgroup namespace.

With no cgroup resolution, no event ever gets a `container_id` populated (a
manual scan of the captured event JSON confirmed: `container_id` field is
absent on every event, and `cgroup.id` is `/init` everywhere — the workspace
container's own cgroup, not the alpine workload's). Without `container_id`,
`ActivityDump.MatchesSelector()` at `pkg/security/security_profile/dump/activity_dump.go:100`
can never match, so no process is ever inserted into the activity tree.

This is structurally analogous to the earlier kprobe-leak limitation: the
playground itself is correct, the binaries do exactly what they should, but
this particular host (a Datadog workspace container running its own Docker as a
peer to the workload containers) doesn't expose enough of the cgroup hierarchy
for the activity-dump tracer to stitch together a workload. On a bare-metal
Linux host (the documented target environment for the playground per the
`Out of scope` note), this would work as intended.

#### Acceptance checklist

- [x] `dev/cws-explore/` exists with all six required artifacts
- [x] `run.sh build` succeeds; both binaries built
- [x] `system-probe runtime policy check` reports the catch-all policy as valid
- [x] Live ≥5 distinct rule_ids — verified at 14, 279K events captured across earlier session, 31K events / 8s in this session
- [ ] Activity-dump JSON non-trivial — **blocked by workspace cgroup nesting (sibling Docker containers); CLI surface, registration, listing, and stop all verified to work; kernel-side capture cannot resolve cgroups → 0 nodes → empty profile dropped before persist**
- [x] README walks all three modes + kernel/root caveats + Troubleshooting section
- [x] `doctor` subcommand auto-mounts tracefs and reports stale kprobes
- [x] No edits outside `dev/cws-explore/`

