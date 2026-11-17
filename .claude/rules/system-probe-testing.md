---
paths:
  - "pkg/ebpf/**"
  - "pkg/network/**"
  - "pkg/collector/corechecks/ebpf/**"
  - "cmd/system-probe/**"
  - "pkg/system-probe/**"
  - "pkg/util/kernel/**"
  - "pkg/process/monitor/**"
---

# Testing system-probe and eBPF changes

Read `pkg/ebpf/AGENTS.md` before testing a change here or claiming it works.

- Linux and eBPF code needs a Linux kernel; Windows system-probe code is tested on Windows. macOS can
  only typecheck.
- Report each build mode and kernel as `passed`, `failed`, `blocked`, or `inconclusive`; "compiles" is
  never "tested".
