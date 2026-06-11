# Bazel deb → omnibus deb functional parity plan

Make `bzl build //packages/agent/linux:debian` produce a deb that is a
drop-in replacement for the omnibus-produced deb, enabling `dda inv omnibus.build`
to be wired to Bazel and ultimately allowing omnibus retirement.

---

## Methodology note

This plan follows the same comparison-gate methodology as the installer package
migration (`packages/installer/MIGRATION_PLAN.md`).

**Important correction to the initial gap list:** several items listed as `# TODO`
in `packages/agent/product/BUILD.bazel:43-54` are already wired through
`packages/agent/dependencies/BUILD.bazel` and included transitively via
`//packages/agent/dependencies:all_files`. The ground-truth gap list comes from
actually building the deb and running the gate — Phase 1 is the first step for
exactly this reason.

---

## Gap inventory (static analysis — to be validated in Phase 1)

### A. Already present — verify, don't re-implement

These are in `packages/agent/dependencies/BUILD.bazel` or elsewhere already:
- cacerts (`dependencies/BUILD.bazel:17` → `//deps/cacerts:all_files`) — **stale `# TODO`**
- jmxfetch, snmp_traps, gstatus, nfsiostat (`dependencies/BUILD.bazel:18-19,36,37`)
- Native C deps (curl, krb5, openscap, freetds, libpcap, systemd) (`dependencies/BUILD.bazel:63-94`)
- rtloader + libdatadog-agent-three.so + python shared lib (`product/BUILD.bazel:34`)
- Security agent policies (`dependencies/BUILD.bazel:32-34`)
- Discovery rust + procmgr rust (`product/BUILD.bazel:56-63`)
- agent, trace-agent, process-agent, privateactionrunner, trace_loader (`product/BUILD.bazel:35-43`)
- installer symlinks (`linux/BUILD.bazel:166-212`)

### B. Missing binaries — already build under Bazel, packaging flip only

Add `pkg_files` (mode 0755, prefix `embedded/bin`) to `product/BUILD.bazel`,
same pattern as `dda_built_trace_agent_binary` at lines 162-169:
- `//cmd/system-probe:system-probe`
- `//cmd/security-agent:security-agent`
- `//cmd/cws-instrumentation:cws-instrumentation`

### C. Missing binary — no Bazel build target yet

`cmd/secret-generic-connector/` has `main.go` but no `go_binary` in BUILD.bazel.
Two sub-steps:
1. Author `cmd/secret-generic-connector/BUILD.bazel` with `go_binary`
2. Package it: `pkg_files` at mode **0500** (per `omnibus/config/projects/agent.rb:391`)
3. Ship its LICENSE: `pkg_files` into `LICENSES/secret-generic-connector-LICENSE`

### D. Missing — embedded Python interpreter + stdlib + integrations (LONG POLE)

**Interpreter/stdlib:** `rtloader` packages only the python *shared lib* via `three_pkg`.
The full interpreter/stdlib is only in dev-only `//rtloader:python_env_transitive` (not
in `agent_components`). Fix: promote a non-dev variant into `agent_components`, mirroring
the `dd_collect_dependencies` + `pkg_filegroup` pattern.

**Integrations-py3:** `datadog-agent-integrations-py3.rb` pip-installs wheels from a cache
into `embedded/lib/pythonX.Y/site-packages`, plus `conf.d/<check>.d/` example configs,
SNMP profiles, requirements files. There is no `@integrations_py3` Bazel repo or target.
This is a build-system design spike — the wheel-set needs either a new repo rule or a
prebuilt artifact. **This is the critical-path blocker for omnibus retirement.**

### E. Missing — configuration files / example configs

- `system-probe.yaml.example` (from `bin/agent/dist/`, `datadog-agent.rb:214`)
- `security-agent.yaml.example` (from `pkg/config/example/`, `:259`)
- `application_monitoring.yaml.example` (`:308`)
- `runtime-security.d/`, `compliance.d/` config trees (`finalize.rb:85-86`)
- selinux policy for system-probe (`datadog-agent.rb:209-212` → `etc/datadog-agent/selinux/`).
  Compiled via a selinux tool — no current Bazel coverage; needs `genrule` or prebuilt.

### F. Missing — eBPF assets (system-probe runtime)

`datadog-agent.rb:218-238` installs to `embedded/share/system-probe/`:
- `ebpf/*.o`, `co-re/*.o`, `runtime/*.c` (built by `//pkg/ebpf/bytecode:*` targets)
- `co-re/btf/minimized-btfs.tar.xz`
- `clang-bpf`, `llc-bpf`, `COPYING`

eBPF objects have partial Bazel coverage (`pkg/ebpf/bytecode/BUILD.bazel`) but no packaging
filegroup assembles them into `embedded/share/system-probe/`. clang-bpf / llc-bpf / btf are
likely prebuilt-dep repo rules to add.

### G. Missing — files outside install_dir

Currently unhandled (`finalize.rb` / `agent.rb:259-261`):
- `/usr/bin/dd-agent` symlink (`finalize.rb:71`) → `dd_agent_pkg_mklink`
- `/var/log/datadog/` empty dir (`finalize.rb:98`) → `pkg_mkdirs`
- `processes.d/` dir (`finalize.rb:101`) → `pkg_mkdirs`
- `.install_root` marker (`datadog-agent.rb:313`) → `pkg_files` from generated file
- `python-scripts/` (`datadog-agent.rb:337-345`)
- `embedded/.installed_by_pkg.txt` (`finalize.rb:119-120`) — content couples to
  site-packages listing, so depends on category D

### H. License aggregation

`package_licenses` aspect is already wired (`linux/BUILD.bazel:58`). Outstanding:
- Aggregated top-level `LICENSE` file — installer MIGRATION_PLAN.md marks as pending decision
- `version-manifest.json` / `.txt` — accepted drop (installer plan §lines 131-132, 307-309)

### I. Subtractive parity — finalize.rb strips Bazel currently over-includes

These are omnibus *deletions* from `embedded/` that Bazel currently ships:
- Docs/terminfo/aclocal/examples/man/gtk-doc/info/locale/pkgconfig/cmake (`finalize.rb:55-64,123-145,167`)
- `lib/*.la`, `lib/libdbus-1.a`, `include/dbus-1.0`, `bin/pg_config`, `embedded/include/systemd`
- `__pycache__`/`.pyc` and `site-packages/tests/` (`finalize.rb:114,149`)
- Test/debug eBPF `.o` files (`finalize.rb:153-159`)
- Dedup-symlinks and rpath-edit (`finalize.rb:170`) — changes modes/symlink status, **invisible to tar t; requires `tar tv` to catch**

Best approach: ensure `deps/*` source filegroups exclude these rather than post-strip.
Dedup-symlinks is the hardest to match and may become an explicitly accepted diff.

---

## Comparison gate fixes (gate exists at `.gitlab/build/packaging/comparison-gate.yml`)

The gate exists but must be corrected before results are meaningful:

1. **Switch `tar t` → `tar tv`** (lines 60-63) so mode/ownership/symlink diffs are visible.
   Category-I dedup/rpath changes and category-C chmod 0500 are invisible to name-only diff.

2. **Add `md5sums` comparison**: `dpkg-deb -e` already extracts the control dir; diff `md5sums`
   to catch content drift (rpath edits, dedup). Neither the gate nor MIGRATION_PLAN.md §A
   currently does this.

3. **Scriptlets must diff to zero** — shared files under `//omnibus:package-scripts/agent-deb/`.
   Any nonzero scriptlet diff is a bug.

4. **Control-section diffs to fix (not accept):**
   - deb epoch: `linux/BUILD.bazel:118` has `version = "7"`; omnibus emits `Version: 1:7-1`
     (`agent.rb:144` `epoch 1`). Fix: set `version = "1:7"` or `apt` upgrades break.
   - vendor: `linux/BUILD.bazel:105` has `vendor` commented out; omnibus sets
     `Datadog <package@datadoghq.com>` (`agent.rb:143`). Uncomment.

---

## Done criteria

**Not zero-diff.** Drop-in = the gate's `UNEXPECTED` set (after `ACCEPTED_DIFF_PATTERNS`) is empty.

Accepted differences (carried from installer migration):
- `version-manifest.json`, `version-manifest.txt` — omnibus artifact only
- Aggregated `LICENSE` — pending decision

Must-fix (not acceptable):
- deb epoch format
- vendor field
- Any scriptlet diff
- Any mode diff except documented dedup-symlink cases

---

## Sequencing

```
Phase 1  Validate (ground-truth gap list)
           └── builds deb, runs corrected gate (tar tv + md5sums)
               confirms/prunes stale A items, gets real diff

Phase 2  Quick wins (parallelisable)
           ├── Cat B: 3 binary flips (system-probe, security-agent, cws-instr)
           ├── Control fixes (epoch, vendor)
           ├── Cat G: out-of-install_dir files (mklink, mkdirs, markers)
           └── Cat E: example configs (except selinux)

Phase 3  Subtractive parity (Cat I)
           └── match finalize.rb strips so Bazel stops over-including
               (do after Phase 2 so diff is readable)

Phase 4  Build-gap items (parallelisable with 2-3)
           ├── Cat C: secret-generic-connector go_binary + packaging
           ├── Cat F: eBPF assets + clang/llc/btf prebuilt repos
           └── Cat E: selinux policy compilation (genrule or prebuilt)

Phase 5  Critical-path long pole  ← gates omnibus retirement
           ├── Cat D.1: embedded Python interpreter/stdlib (promote python_env_transitive)
           └── Cat D.2: integrations-py3 wheel mechanism (design spike)

Phase 6  Cutover (mirrors installer MIGRATION_PLAN.md Phase 7)
           ├── gate flips from allow_failure:true to blocking
           ├── parallel Bazel + omnibus packaging jobs in CI
           ├── install-test on Ubuntu/RHEL
           ├── replace omnibus jobs
           └── deprecate → delete omnibus agent .rb files (one release cycle)
```

**Critical path to omnibus retirement:** Phase 1 → Phase 5 (D.2 integrations-py3 is the
gating long pole). Phases 2-4 are parallelisable and incremental behind the `allow_failure`
gate. Nothing blocks on them except Phase 6 cutover readiness.

---

## Key files

| File | Role |
|------|------|
| `packages/agent/product/BUILD.bazel` | Add Cat B/C/E/F `pkg_files` entries |
| `packages/agent/linux/BUILD.bazel` | Control-section fixes (epoch, vendor); collect new components |
| `packages/agent/dependencies/BUILD.bazel` | Source for already-present items (verify Cat A) |
| `.gitlab/build/packaging/comparison-gate.yml` | Fix `tar t` → `tar tv`; add md5sums diff |
| `cmd/secret-generic-connector/BUILD.bazel` | New file: `go_binary` target (Cat C) |
| `rtloader/BUILD.bazel` | Source of `python_env_transitive` to promote (Cat D.1) |
| `omnibus/config/software/datadog-agent.rb` | Authoritative install list |
| `omnibus/config/software/datadog-agent-finalize.rb` | Authoritative strip/transform list |
| `packages/installer/MIGRATION_PLAN.md` | Process template to follow exactly |
