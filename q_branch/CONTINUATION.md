⏺ ## Continuation: fine-grained-monitor Implementation

  ### Context
  We completed spEARS specification for `q_branch/fine-grained-monitor/` - a Rust tool to capture fine-grained container metrics (PSS, CPU, cgroup) to
  Parquet. Solves "who watches the watcher" for Datadog Agent development.

  **Specs location:** `q_branch/fine-grained-monitor/specs/container-monitoring/`
  - requirements.md: 5 EARS requirements (REQ-FM-001 through REQ-FM-005)
  - design.md: Architecture, component design
  - executive.md: Status tracking (0/5 complete)

  ### Key Decisions Made
  1. **Dependencies:** `lading_capture` + `lading_signal` from git (CaptureManager API is clean)
  2. **Vendor:** Observer code from lading (Sampler is `pub(crate)`)
  3. **Container discovery:** Cgroup filesystem scan (`/sys/fs/cgroup/kubepods*`)
  4. **Memory focus:** PSS over RSS; smaps gated behind `--verbose-perf-risk` (mm lock)
  5. **Safety:** 1 GiB parquet file size limit

  ### Implementation Order
  1. **REQ-FM-004** - `lading_capture` integration (CaptureManager, parquet output)
  2. **REQ-FM-001** - Container discovery via cgroup scan
  3. **REQ-FM-002/003** - Vendor procfs/cgroup parsers from lading

  REQ-FM-005 (delayed metrics) is ⏭️ Planned for later phase.

  ### Items to Verify During Implementation
  - `smaps_rollup` availability for PSS (design assumes it exists)
  - Cgroup path patterns on KIND cluster (`cri-containerd-*.scope`)
  - File size monitoring logic (not built into lading_capture)
  - Cargo.toml git deps may need pinning

  ### Dev Environment
  - Lima VM: `limactl shell gadget-k8s-host`
  - KIND cluster: `gadget-dev`
  - Test target: DatadogAgent CR in `q_branch/test-cluster.yaml`

  ### Start Point
  Read `specs/container-monitoring/executive.md` for current status, then begin REQ-FM-004 implementation.
