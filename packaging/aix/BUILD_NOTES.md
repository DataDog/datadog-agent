# AIX 7.2 Build Notes — Agent 7.80.0

## Environment

| Item | Value |
|------|-------|
| Target | AIX 7.2 TL2 (soaix431) |
| Host | cloud3.siteox.com:43122 |
| Agent source on VM | `/dd/datadog-agent` |
| Build artifacts | `/opt/dd-build/` |
| Agent version | 7.80.0 |
| Build number | 1 |

## Stages

### Stage 00 — Checkout

**Status:** ✅ Complete (idempotent re-run)

`build.sh` hardcoded `/opt/datadog-agent` for the `.git` check and `agent.version`
call, but the source lives at `/dd/datadog-agent`. Also, `00-checkout.sh` hardcoded
`AGENT_SRC=/opt/datadog-agent` rather than honouring the env var.

**Fixes applied:**
- `packaging/aix/build.sh`: Accept `AGENT_SRC` env var (default `/opt/datadog-agent`),
  export it, use it for the `.git` check and version auto-detection.
- `packaging/aix/stages/00-checkout.sh`: Change `AGENT_SRC=/opt/datadog-agent` →
  `AGENT_SRC=${AGENT_SRC:-/opt/datadog-agent}`.

**Usage:**
```sh
AGENT_SRC=/dd/datadog-agent AGENT_VERSION=7.80.0 AGENT_BUILD=1 ./build.sh
```

---

### Stage 01 — Native libs

**Status:** ✅ Complete (sentinel present, skipped)

---

### Stage 02 — Python 3.13

**Status:** ✅ Complete (sentinel present, skipped)

---

### Stage 03 — rtloader

**Status:** ✅ Complete (sentinel present, skipped)

---

### Stage 04 — Agent binary

**Status:** 🔄 In progress (2nd attempt with perfstat fix)

**Root cause:** `github.com/power-devops/perfstat` (indirect dep via gopsutil) calls
`perfstatdiskadapter2diskadapter()` in `helpers.go`, which accesses 11 fields of
`perfstat_diskadapter_t` that only exist from `CURR_VERSION_DISKADAPTER >= 3`. On AIX 7.2
TL2, `CURR_VERSION_DISKADAPTER=2` and those fields are absent from the C struct, causing
compilation to fail:
```
n.min_rserv undefined (type *_Ctype_perfstat_diskadapter_t has no field or method min_rserv)
... (10 more similar errors)
```

Missing fields: `min_rserv`, `max_rserv`, `min_wserv`, `max_wserv`, `wq_depth`,
`wq_sampled`, `wq_time`, `wq_min_time`, `wq_max_time`, `q_full`, `q_sampled`.

Note: `perfstat_disk_t` and `perfstat_diskpath_t` are unaffected — both already include
all referenced fields on AIX 7.2.

**Fix:**
- Added `third_party/perfstat/` — a local fork of `power-devops/perfstat@v0.0.0-20240221224432-82ca36839d55`
- `c_helpers.c`: Added `DA_FIELD_GETTER` macro guarded by `#if CURR_VERSION_DISKADAPTER >= 3`
  that returns the real field value on AIX 7.3+ or `0` on AIX 7.2.
- `c_helpers.h`: Added `extern` declarations for the 11 `da_get_*` getter functions.
- `helpers.go`: Replaced direct `n.<field>` accesses with `C.da_get_<field>(n)` calls.
- `go.mod`: Added `replace github.com/power-devops/perfstat => ./third_party/perfstat`.

---

### Stage 04 — Agent binary (gcc-8 wrapper fix)

**Additional fix applied:** Go's linker probes `gcc-8 -Wl,-V` to detect the external linker, with no
flags. `OBJECT_MODE=64` puts `ld` in 64-bit mode but gcc-8 still links the 32-bit
`/lib/crt0.o` startup file, causing `ld` to reject it. Fix: create wrapper scripts
in `$BUILD_DIR/gcc-wrap/` that always inject `-maix64`, and set `CC`/`CXX` to
those wrappers in `env.sh`.

---

### Stage 05 — Python C extensions

**Status:** ✅ Complete (second run, with wheel cache fix)

cffi, psutil, lxml all compiled from source cleanly.

**cryptography** requires Rust (via `maturin`). IBM Rust SDK 1.92 cannot run on AIX 7.2 TL2
because `libc++.so.1` (from XL C++ 16.1.0.10, needed by `shr2_64.o`, needed by `rustc.orig`)
references `strftime_l` which is only available from AIX 7.2 TL3+. This is the same TL2
limitation that requires GCC 8 for the agent binary.

**Fix:** build cryptography on AIX 7.3 (where Rust SDK works), verify the built
`_rust.abi3.so` only imports `libc.a[shr_64.o]` (no C++ runtime, no `strftime_l`),
retag the wheel from `aix_3_00F9D80F4C00` → `aix_7202_2015_64`, and populate the wheel
cache. Stage 05 then finds the cached wheel and installs it without invoking Rust.

---

### Stage 06 — pydantic-core (Rust)

**Status:** ✅ Complete (from wheel cache)

Same Rust SDK issue as stage 05. Same fix: copy `pydantic_core-2.41.5` wheel from AIX 7.3,
retag to `aix_7202_2015_64`, place in `$WHEEL_CACHE/pydantic-2.12.5/`. The `_pydantic_core.cpython-313.so`
imports only `libc.a[shr_64.o]`, `libpthread.a`, `libunwind.a`, `libpython3.13.a`, `libbsd.a` —
no C++ runtime, no `strftime_l`.

---

### Stage 07 — datadog-checks-base deps

**Status:** ✅ Complete

`jellyfish==1.2.1` requires Rust — same issue as cryptography/pydantic. Built on AIX 7.3
(~2 min), retagged to `aix_7202_2015_64`, placed in `$BUILD_DIR/aix72-wheels/`.

**Fix in script:** Added `--find-links $BUILD_DIR/aix72-wheels` to the pip install command
so any Rust-based wheel can be pre-populated there without modifying the install logic.

---

### Stage 08 — integrations

**Status:** ✅ Complete

No additional Rust packages required. All checks installed cleanly.

---

### Stage 09 — strip bytecode

**Status:** ✅ Complete

---

### Stage 10 — assemble

**Status:** ✅ Complete (after AGENT_SRC fix)

Hardcoded `/opt/datadog-agent` paths for:
- `cmd/agent/dist/datadog.yaml` (config example)
- `bin/agent/dist/conf.d` (check configs built by stage 04)

**Fix:** Added `AGENT_SRC=${AGENT_SRC:-/opt/datadog-agent}` and replaced both hardcoded
paths with `$AGENT_SRC/...`.

---

### Package (mkinstallp)

**Status:** ✅ Complete

**Output:** `/opt/dd-build/datadog-agent-7.80.0-1.aix.ppc64.bff` (~264 MB)

mkinstallp produced non-fatal `0503-880` warnings for files already present from a
previous agent install on the build host — these do not affect the package contents.

---

## Making IBM Rust SDK 1.92 run natively on AIX 7.2 TL2

IBM Rust SDK 1.92 cannot run on AIX 7.2 TL2 out of the box because
`librustc_driver` loads `libc++.so.1` from XL C++ 16.1.0.10, which references
`strftime_l` — absent from `libc.a[shr_64.o]` until TL3. The fix is a
**surgical binary patch to `libc++.so.1`** to import `strftime_l` from our own
stub library instead of from `libc.a[shr_64.o]`.

### XCOFF loader-section patch (`third_party/perfstat/` analogue)

The `libc++.so.1` member inside `/usr/lpp/xlC/lib/libc++.a` has an XCOFF64
loader section with an import symbol table. Symbol [185] (`strftime_l`) has
`l_ifile = 2` pointing to import file entry 2 (`libc.a / shr_64.o`).

The patch (`/tmp/patch_xcoff_final.py`):
1. Appends a new import file entry (index 5) to the import string table:
   `\x00libzz.a\x00strfm.o\x00` (17 bytes, same delta as the insertion)
2. Increments `l_nimpid` (5 → 6) and `l_istlen` (0x75 → 0x86)
3. Shifts `l_stoff` (export string table) by the delta
4. Updates the loader section size in the XCOFF section header
5. Updates `f_symptr` in the XCOFF file header
6. Changes symbol [185] `strftime_l`'s `l_ifile` from 2 to 5 (new entry)

The patched `libc++.so.1` now imports `strftime_l` from
`/opt/freeware/lib/libzz.a[strfm.o]` instead of `libc.a[shr_64.o]`.

### Stub library `/opt/freeware/lib/libzz.a[strfm.o]`

```c
#include <time.h>
typedef void *locale_t;
size_t strftime_l(char *s, size_t max, const char *fmt,
                  const struct tm *tm, locale_t loc) {
    (void)loc;
    return strftime(s, max, fmt, tm);
}
```

AIX locale is process-global, so ignoring `locale_t` is correct — identical
to AIX 7.3's own `strftime_l` implementation (same 40-byte body as `strftime`).

### Additional library members needed

With `$EMBEDDED_DESTDIR/lib` first in LIBPATH (required to preserve Python
SSL with embedded OpenSSL), two more shared members are needed in the embedded
archives so Rust tools find them there rather than falling through to the
system-path versions that may not exist or conflict:

| Archive | Member added | Source |
|---------|-------------|--------|
| `libz.a` | `libz.so.1` (64-bit) | `/opt/freeware/lib/libz.a` |
| `libssl.a` | `libssl.so.3` (64-bit) | `/opt/freeware/lib/libssl.a` |
| `libcrypto.a` | `libcrypto.so.3` (64-bit) | `/opt/freeware/lib/libcrypto.a` |

Also added `libc++abi.so.1` and `shr2_64.o` to `/usr/lpp/xlC/lib/libc++.a`
from AIX 7.3 (needed by `librustc_driver`).

### Additional packaging needed

| Package | Issue | Fix |
|---------|-------|-----|
| `maturin>=1,<2` | `cargo metadata` SIGSEGV on AIX 7.2 when resolving its large dep graph | Pre-built wheel from AIX 7.3, retagged `aix_7202_2015_64`, in `$BUILD_DIR/aix72-wheels/` |
| `jellyfish==1.2.1` | Same cargo crash | Same approach |

Env vars required for Rust build stages (05, 06, 07):
```sh
LIBPATH=$EMBEDDED_DESTDIR/lib:/opt/freeware/lib:${LIBPATH:-}
TMPDIR=$BUILD_DIR/gotmp                         # /tmp is only 3GB
CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse      # full git index causes SIGSEGV
```

---

## Why older IBM Rust SDK versions don't help

IBM publishes Rust SDK RPMs explicitly labeled `aix7.2.ppc.rpm` — versions 1.86, 1.88,
1.90, 1.92, and 1.94 are all available at:
`https://public.dhe.ibm.com/aix/freeSoftware/aixtoolbox/RPMS/ppc-7.2/rust/`

However, all of these versions (at minimum 1.86+) have the same `strftime_l` problem.
The `librustc_driver-*.a` in each version requires `libc++.a[shr2_64.o]`, which in turn
depends on `libc++.so.1`, which references `strftime_l`. This was confirmed by inspecting
the XCOFF loader section of `librustc_driver` from a downloaded rust1.86 RPM via
`dump -X64 -H` on the AIX 7.2 VM:

```
4   libc++abi.a  libc++abi.so.1
5   libc++.a     shr2_64.o        ← requires strftime_l transitively
```

The `aix7.2.ppc.rpm` label means "runs on AIX 7.2" but IBM implicitly requires TL3+
for Rust. The cross-build approach (build on AIX 7.3, verify `.so` symbols, retag
`aix_7202_2015_64`) is the correct long-term workaround until IBM provides a Rust SDK
explicitly certified for AIX 7.2 TL2.

**Note:** A `strings`-based analysis of the rustc binary on Linux falsely showed `shr_64.o`
as a dependency — Linux `strings` cannot parse XCOFF loader sections. Always use
`dump -X64 -H` on AIX to verify actual runtime dependencies.

---

## Summary of all fixes

| Area | Fix |
|------|-----|
| `build.sh` | Added `AGENT_SRC` env var support (default `/opt/datadog-agent`) |
| `00-checkout.sh` | Use `${AGENT_SRC:-/opt/datadog-agent}` instead of hardcoded path |
| `env.sh` — GCC wrapper | Wrap `gcc-8`/`g++-8` to always inject `-maix64`; Go's linker probe otherwise links 32-bit `crt0.o` in 64-bit mode |
| `go.mod` | Add `replace github.com/power-devops/perfstat => ./third_party/perfstat` |
| `third_party/perfstat` | CGo `#if CURR_VERSION_DISKADAPTER >= 3` wrappers for 11 fields missing from AIX 7.2's `perfstat_diskadapter_t` |
| `tasks/build_tags.py` | (Not needed — `zookeeper.go` already uses `github.com/go-zookeeper/zk` in HEAD) |
| AIX 7.2 XL C++ | Inject `shr2_64.o` and `libc++.so.1` from AIX 7.3's `libc++.a` into 7.2's to satisfy Rust SDK deps |
| `/home` filesystem | Extended from 3 GB → 32 GB via `extendlv hd1 464 && chfs -a size=32768M /home` |
| Rust wheels (stages 05-07) | `cryptography`, `pydantic_core`, `jellyfish` built on AIX 7.3, retagged `aix_7202_2015_64`, pre-populated in wheel caches |
| `07-checks-base.sh` | Added `--find-links $BUILD_DIR/aix72-wheels` for future Rust packages |
| `10-assemble.sh` | Replace hardcoded `/opt/datadog-agent` paths with `$AGENT_SRC` |
