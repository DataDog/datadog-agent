---
name: cws-local-e2e
description: Verify a CWS (pkg/security) change end to end on this host — real system-probe binary, real config, real serialized event — by running it against a local intake sink and asserting on the event it sends.
allowed-tools: Bash, Read, Write, Edit
---

# CWS local end-to-end check

`dda inv security-agent.local-e2e-up`, `-wait` and `-down` run the locally built
system-probe against an HTTP sink on 127.0.0.1 and assert on the event it would
have sent to Datadog. Read each task's `--help` for flags. It needs passwordless
`sudo` and eBPF; the dev container works.

What `--help` does not tell you:

- **Scope the rule to your trigger** (a unique path, process name or argument),
  and assert on the field your change is about, not only on the rule firing.
  Add an `--absent` check when the change could over-match.
- **`--no-build` can test a stale binary**, which then passes for the wrong reason.
  Only use it when the binary and eBPF objects already contain your change.
- **`--expect` paths are into the serialized event, not SECL field names**: SECL
  `process.file.name` is `process.executable.name`, `open.flags` is `file.flags`
  (a list of names). Run `wait` without `--expect` first and read the event.
- **The trigger runs as you**; prefix it with `sudo` when the scenario needs root.
- **The harness disables self-tests, activity dumps, security profiles and remote
  configuration**, so only your policy produces events. Turn them back on with
  `--override` when they are what you test. `--override` only edits
  `system-probe.yaml`; keys read from `datadog.yaml` are not covered.
- **One kernel only.** A pass says nothing about other kernels (verifier limits,
  missing BTF offsets); behaviour that must hold across kernels needs a functional
  test, which CI runs on the KMT matrix.
- The dev shell is itself a container, so events carry a top-level `container`
  even for "host" activity.

Everything lives in `/run/cws-local-e2e/`: `system-probe.log`, the generated
configs, and `events.jsonl` (every event the sink received, one record per line,
for `jq`). `tagger`/`workloadmeta` "unable to establish stream" warnings are
expected without a core agent. Do not run two `dda inv` commands at once: they
race on a shared temporary binary.
