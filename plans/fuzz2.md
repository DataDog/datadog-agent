# Fuzzing Campaign — Wave 2: /proc and /sys Parsing

## Context

Following the first wave (which found and fixed 3 crashes via fuzzing in `pkg/process/procutil` and `pkg/util/cgroups`), this document identifies 5 new targets from a broader survey of `/proc`/`/sys` parsing code. Targets are ordered by confidence in triggering a real panic.

Previously fixed (skip):
- `parseStatContent` – nil `bootTime` pointer
- `parseV2IOFn` (`cgroupv2_io.go:66`) – missing `return nil` after `reportError`
- `parseAddress` (`usm/procnet/parser.go:171`) – `binary.BigEndian.Uint16(buf[:0])`

---

## Target 1 — `IntPair.get()` · `pkg/network/config/sysctl/sysctl.go` ✦ CONFIRMED CRASH

**Reads:** `/proc/sys/net/ipv4/ip_local_port_range`

**Crash:** Lines 143 and 145 index `vals[0]` and `vals[1]` on `strings.Fields(v)` with **no bounds check**:
```go
vals := strings.Fields(v)
i.v1, err = strconv.Atoi(vals[0])   // line 143 – panic if len(vals) == 0
if err == nil {
    i.v2, err = strconv.Atoi(vals[1])   // line 145 – panic if len(vals) == 1
}
```
- Empty file → `strings.Fields("")` = `[]string{}` → `vals[0]` panics
- Single field `"1024\n"` → `vals[1]` panics

**Call chain:** `sysctl.NewIntPair(procRoot, "net/ipv4/ip_local_port_range", ...)` → reads `filepath.Join(procRoot, "sys", "net/ipv4/ip_local_port_range")` → `IntPair.Get()` → `IntPair.get()` → crash.

**Fuzz harness:** `FuzzIntPairGet` in `pkg/network/config/sysctl/sysctl_fuzz_test.go`
- Create temp dir with `sys/net/ipv4/` subdirectory
- Write fuzzer-controlled content to `ip_local_port_range`
- Call `sysctl.NewIntPair(tmpDir, "net/ipv4/ip_local_port_range", 0).Get()`
- Crash seeds: `[]byte("")`, `[]byte("1024\n")`

**Key files:**
- `pkg/network/config/sysctl/sysctl.go` (lines 139–150)
- `pkg/network/ephemeral_linux.go` (lines 25–37)
- `pkg/network/config/sysctl/sysctl_test.go` — existing tests show setup pattern

---

## Target 2 — `parseMountinfo()` · `pkg/util/containers/metrics/system/containerid_linux.go` ✦ SUSPECTED

**Reads:** `/proc/self/mountinfo`

**Suspected crash:** Lines 52–53 access `matches[1]` and `matches[2]` with insufficient guard:
```go
matches := allMatches[len(allMatches)-1]
if len(matches) > 0 && matches[1] != containerdSandboxPrefix {
    return matches[2], nil   // panics if len(matches) < 3
}
```
The check is `len(matches) > 0` but code accesses indices 1 and 2. The regex has 2 explicit capture groups so a full match always produces 3 elements — but fuzz to confirm no partial-match edge case escapes.

**Fuzz harness:** `FuzzParseMountinfo` in `pkg/util/containers/metrics/system/containerid_linux_fuzz_test.go`
- `parseMountinfo` is unexported; fuzz via the package-level function in the same package
- Write fuzzer-controlled content to a temp file; call `parseMountinfo(tmpFile)`
- Crash seeds: `[]byte("")`, a line that partially matches the regex pattern

**Key files:**
- `pkg/util/containers/metrics/system/containerid_linux.go` (lines 18–59)

---

## Target 3 — `GetMemoryStats()` · `pkg/util/cgroups/cgroupv1_memory.go` ✦ COVERAGE

**Reads:** `/sys/fs/cgroup/memory/memory.stat`, `memory.limit_in_bytes`, `memory.usage_in_bytes`, etc.

**No confirmed panic** — all parsing delegated to safe `parse2ColumnStats` / `parseSingleUnsignedStat` helpers. Coverage target to confirm cgroupv1 is as robust as cgroupv2 and guard against future regressions.

**Fuzz harness:** `FuzzGetMemoryStats` in `pkg/util/cgroups/cgroups_fuzz_test.go` (append to existing file)
- Use `cgroupMemoryFS` + `createCgroupV1` + `setCgroupV1File` from `file_for_test.go`
- Set fuzzer-controlled content for `memory.stat`
- Call `cg.GetMemoryStats(stats)`

**Key files:**
- `pkg/util/cgroups/cgroupv1_memory.go`
- `pkg/util/cgroups/file_for_test.go` — `cgroupMemoryFS`, `createCgroupV1`, `setCgroupV1File`

---

## Target 4 — `GetCPUStats()` · `pkg/util/cgroups/cgroupv2_cpu.go` ✦ COVERAGE

**Reads:** `/sys/fs/cgroup/cpu.stat`, `cpu.max`, `cpu.weight`, `cpu.pressure`, `cpuset.cpus.effective`

**No confirmed panic** — all parsing via helpers. `cpuset.cpus.effective` goes through `ParseCPUSetFormat` (already fuzzed by `FuzzParseCPUSetFormat`). Integration-level coverage across all five file paths simultaneously.

**Fuzz harness:** `FuzzGetCPUStats` in `pkg/util/cgroups/cgroups_fuzz_test.go` (append to existing file)
- Use `cgroupMemoryFS` + `createCgroupV2` + `setCgroupV2File`
- Set fuzzer-controlled content for `cpu.stat` and `cpu.max`
- Call `cg.GetCPUStats(stats)`

**Key files:**
- `pkg/util/cgroups/cgroupv2_cpu.go` (lines 17–120)
- `pkg/util/cgroups/file_for_test.go` — `createCgroupV2`, `setCgroupV2File`

---

## Target 5 — `readProcNetWithStatus()` · `pkg/network/proc_net.go` ✦ COVERAGE

**Reads:** `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`

**No confirmed panic** — `fieldIterator.nextField()` safely returns empty slices; the one risky index (`rawLocal[idx+1:]`) is guarded by `if idx == -1 { continue }`. Coverage target parallel to the already-fuzzed USM `FuzzNewEntry` harness; guards against divergence between the two copies of `fieldIterator`.

**Fuzz harness:** `FuzzReadProcNetWithStatus` in `pkg/network/proc_net_fuzz_test.go`
- Write fuzzer-controlled bytes to a temp file
- Call `readProcNetWithStatus(tmpFile, 10)` (status 10 = tcpListen)
- Seeds: valid `/proc/net/tcp` header + data lines; empty file; malformed lines

**Key files:**
- `pkg/network/proc_net.go` (lines 33–130)
- `pkg/network/proc_net_test.go` — existing tests show the pattern

---

## Priority

| Target | Confidence | Expected result on first seed run |
|--------|------------|----------------------------------|
| 1. sysctl `IntPair.get()` | **High — confirmed crash** | Seed fails immediately: `index out of range` |
| 2. `parseMountinfo` | Medium — suspected | May fail; fuzz to confirm |
| 3. cgroupv1 `GetMemoryStats` | Low — coverage | Seeds likely pass |
| 4. cgroupv2 `GetCPUStats` | Low — coverage | Seeds likely pass |
| 5. `readProcNetWithStatus` | Low — coverage | Seeds likely pass |

---

## Build Tags

| Package | Build tag | Test invocation |
|---------|-----------|-----------------|
| `pkg/network/config/sysctl` | `linux` | `-tags=test` |
| `pkg/util/containers/metrics/system` | none | `-tags=test` |
| `pkg/util/cgroups` | `linux` | `-tags=test` |
| `pkg/network` | `linux` | `-tags=test` |

---

## Verification

```bash
# Target 1 — expect FAIL on crash seed, PASS after fix:
go test -run='FuzzIntPairGet' ./pkg/network/config/sysctl/ -tags=test -v

# Targets 2–5 — run seeds (may or may not fail):
go test -run='FuzzParseMountinfo' ./pkg/util/containers/metrics/system/ -tags=test -v
go test -run='FuzzGetMemoryStats' ./pkg/util/cgroups/ -tags=test -v
go test -run='FuzzGetCPUStats' ./pkg/util/cgroups/ -tags=test -v
go test -run='FuzzReadProcNetWithStatus' ./pkg/network/ -tags=test -v

# Brief live fuzzing on confirmed crash target after fix:
go test -fuzz=FuzzIntPairGet -fuzztime=30s ./pkg/network/config/sysctl/ -tags=test
```
