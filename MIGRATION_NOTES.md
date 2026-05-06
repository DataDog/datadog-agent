# Go-to-Bazel migration session notes

Branch: `chouquette/bazel/go_migration`
Rebased onto: `origin/jsgette/orphan_packages_migration` (tip `5afbb448f0c`)

## Packages migrated (26)

Removed from `# gazelle:exclude` in root BUILD.bazel:

- cmd/otel-agent/subcommands/status
- cmd/trace-agent/subcommands/controlsvc
- cmd/trace-agent/windows/controlsvc
- comp/core/tagger/k8s_metadata
- comp/core/workloadfilter/program
- comp/otelcol/ddprofilingextension/fx
- comp/process/profiler/fx
- comp/process/profiler/impl
- pkg/dyninst/dispatcher
- pkg/dyninst/gotype/gotypeprinter
- pkg/dyninst/process
- pkg/dyninst/symdb/symdbutil
- pkg/dyninst/symdb/uploader
- pkg/dyninst/testprogs/progs/busyloop
- pkg/dyninst/testprogs/progs/drop_tester
- pkg/dyninst/testprogs/progs/fault
- pkg/dyninst/testprogs/progs/simple
- pkg/ebpf/compiler
- pkg/ebpf/names
- pkg/network/ebpf/probes
- pkg/network/usm/buildmode
- pkg/system-probe/api/client
- pkg/system-probe/api/server/testutil
- pkg/util/kubernetes/certificate
- pkg/windowsdriver/ddinjector
- test/e2e-framework/common/config

## Packages attempted but reverted

Re-excluded in root BUILD.bazel after testing revealed issues:

| Package | Reason |
|---------|--------|
| comp/rdnsquerier/fx-mock | Imports `comp/rdnsquerier/mock` which has `//go:build test` — fx-mock library compiles without the tag, gets undefined symbol |
| tools/NamedPipeCmd | `main.go` is `//go:build windows` only — empty package main on Linux fails to link |
| pkg/dyninst/testprogs/progs/rc_tester | Sub-module imports `dd-trace-go/contrib/net/http/v2` not in main go.mod |
| pkg/dyninst/testprogs/progs/rc_tester_v1 | Sub-module imports `gopkg.in/DataDog/dd-trace-go.v1` not in main go.mod |

## New excludes added during regen testing

These pre-existing migrated packages had hand-curated BUILD.bazel files that gazelle's regeneration broke (they reference targets in `# gazelle:ignore` packages). Excluded to keep them as-is:

- comp/core/workloadmeta/collectors/internal/containerd — refs `//pkg/util/containers:containers` (gazelle:ignore)
- comp/core/workloadmeta/collectors/internal/crio — refs `//pkg/sbom/collectors:collectors` (gazelle:ignore)
- pkg/collector/python — refs `//pkg/util/hostname:hostname` (gazelle:ignore)
- pkg/collector/sharedlibrary/sharedlibraryimpl — refs `//pkg/collector/check:check` (gazelle:ignore)
- pkg/dyninst/symdb — refs `pkg/dyninst/object` (no BUILD file)
- pkg/ebpf/bytecode/runtime — refs `//pkg/ebpf:ebpf` (gazelle:ignore)

## Commits

1. `18b2dc0d7e8` — migrate more packages (root BUILD.bazel exclusion edits + new BUILD.bazel files)
2. `ba5053d4c3f` — regenerate BUILD.bazel files (gazelle output for previously-migrated packages, plus the new excludes above)

## Process notes

- Workflow: edit root BUILD.bazel exclusions → `bazel run //:gazelle` → keep new BUILD.bazel files in migration commit, separate gazelle regens into dedicated commit.
- Gazelle regenerations for many already-migrated packages produced broken deps targeting `# gazelle:ignore` packages. Each was reverted and re-excluded individually as discovered via test failures.
- `find_safe_to_migrate.py` (in `~/dd/experimental/teams/agent-supply-chain/go_migration_module_graph/`) lists candidates by score; only direct excludes (path matches a `# gazelle:exclude` line in root BUILD.bazel) are simple to migrate. Parent-covered candidates need exclusion list restructuring.
- Risky source patterns to skip up front: `package main` with single-tag constraint, sub-modules with deps not in main go.mod, mock packages consumed without `gotags = ["test"]`.
