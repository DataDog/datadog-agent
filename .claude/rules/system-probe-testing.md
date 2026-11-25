---
paths:
  - "pkg/ebpf/**"
  - "pkg/network/**"
  - "pkg/collector/corechecks/ebpf/**"
  - "pkg/dyninst/**"
  - "cmd/system-probe/**"
  - "pkg/system-probe/**"
  - "pkg/util/kernel/**"
  - "pkg/process/monitor/**"
---

# Testing system-probe and eBPF changes

Read `pkg/ebpf/AGENTS.md` before testing a change here or claiming it works, and
`pkg/ebpf/TESTING_GOTCHAS.md` before trusting a CI result.

- Typecheck on any host. On a Linux or macOS workstation, develop against local KMT VMs (`kmt.test`).
  KMT isn't supported in workspaces, so there local VMs are `blocked`. Use KMT in CI as the final
  matrix.
- Report each build mode and kernel as `passed`, `failed`, `blocked`, or `inconclusive`; "compiles" is
  never "tested".
