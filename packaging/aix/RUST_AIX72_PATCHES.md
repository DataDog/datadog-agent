# IBM Rust SDK 1.92 on AIX 7.2 TL2 — Complete Patch Inventory

## Problem Statement

IBM Rust SDK 1.92 (`rust1.92-1.92.0-1.ppc` from the AIX Toolbox) cannot load on
AIX 7.2 TL2 out of the box. The dependency chain that breaks is:

```
rustc.orig / cargo.orig
  └── librustc_driver-*.a[librustc_driver.so]   (Rust compiler internals)
        └── libc++.a[shr2_64.o]                  (XL C++ runtime, added in 16.1.0.10)
              └── libc++.a[libc++.so.1]           (new C++ runtime image)
                    └── libc.a[shr_64.o]::strftime_l   ← MISSING on TL2
```

`strftime_l` is a POSIX locale-aware time formatter added to AIX libc in TL3
(APAR IJ17514). On TL2, `libc.a[shr_64.o]` simply does not export it.

The AIX TCB (Trusted Computing Base) prevents loading any `libc.a` from a
non-canonical path, so LIBPATH tricks cannot bypass the missing symbol.

---

## Patch 1 — XCOFF64 Binary Patch of `libc++.so.1`

**File modified:** `/usr/lpp/xlC/lib/libc++.a` (member `libc++.so.1`)

**What it does:** Redirects the `strftime_l` import inside `libc++.so.1` from
`libc.a[shr_64.o]` to a new stub library `libzz.a[strfm.o]` that provides
the function without needing TL3.

**How it works:** XCOFF64 loader sections contain an import symbol table where
each symbol has an `l_ifile` field (4 bytes) pointing to an import file entry.
Symbol [185] (`strftime_l`) had `l_ifile = 2` → `libc.a / shr_64.o`. The patch:

1. Appends a new 17-byte entry to the import string table:
   `\x00libzz.a\x00strfm.o\x00` → becomes import file ID 5
2. Increments `l_nimpid` in the loader header: `5 → 6`
3. Increases `l_istlen` by 17: `0x75 → 0x86`
4. Shifts `l_stoff` (export string table offset) by 17
5. Updates the `.loader` section size in the XCOFF section header
6. Updates `f_symptr` in the XCOFF file header (symbol table is after the loader section)
7. Sets symbol [185] `l_ifile` = `5` (new `libzz.a / strfm.o` entry)

**Script used:** `/tmp/patch_xcoff_final.py` (committed in repo at
`packaging/aix/tools/patch_xcoff_libcxx.py`)

**Key offsets (libc++.so.1 from XL C++ 16.1.0.10 / AIX 7.3):**
- File offset of `.loader` section: `0x11c400`
- Loader header size: 56 bytes (not the typical 48)
- Symbol table starts: `L + 0x38`
- `strftime_l` entry: symbol index 185
- Insert point for new string: `L + 0x2f490 + 0x75 = 0x14b905`

**Important:** the library name `libzz.a` was chosen specifically because
`libs.a` already exists in `/opt/freeware/lib` (used by `libldap`) and caused
the AIX loader to fail looking for `shr_64.o` inside it. Any name that doesn't
conflict with existing libraries works, as long as the total new string entry
is the same length (17 bytes) to keep the XCOFF section sizes consistent.

---

## Patch 2 — `strftime_l` Stub Library

**File created:** `/opt/freeware/lib/libzz.a` (member `strfm.o`)

**Source:**
```c
/* /tmp/strfmt_stub.c */
#include <time.h>
typedef void *locale_t;

size_t strftime_l(char *s, size_t max, const char *fmt,
                  const struct tm *tm, locale_t loc)
{
    (void)loc;
    return strftime(s, max, fmt, tm);
}
```

**Compile command (on AIX 7.2):**
```sh
/opt/dd-build/gcc-wrap/gcc-8 -maix64 -shared -Wl,-brtl -Wl,-bexpall \
    -o /tmp/strfm.o /tmp/strfmt_stub.c
ar -X64 -q /opt/freeware/lib/libzz.a /tmp/strfm.o
```

**Why this is correct:** AIX locale is process-global (`setlocale()` affects
the whole process). The `locale_t` parameter in `strftime_l` is a no-op on
AIX — confirmed by inspecting the AIX 7.3 implementation: `strftime_l` is
exactly 40 bytes (same as `strftime`) and immediately after it in the symbol
table, consistent with being a trivial wrapper.

---

## Patch 3 — Add `shr2_64.o` to `/usr/lpp/xlC/lib/libc++.a`

**File modified:** `/usr/lpp/xlC/lib/libc++.a`

`librustc_driver` imports `libc++.a[shr2_64.o]`. This member exists in XL C++
16.1.0.10 (AIX 7.3) but not in 16.1.0.7 (AIX 7.2 TL2).

**Command:**
```sh
# On AIX 7.3: extract
ar -X64 -x /usr/lpp/xlC/lib/libc++.a shr2_64.o
# Transfer to AIX 7.2, then:
ar -X64 -q /usr/lpp/xlC/lib/libc++.a shr2_64.o
```

**Revert:**
```sh
ar -X64 -d /usr/lpp/xlC/lib/libc++.a shr2_64.o
```

---

## Patch 4 — Add patched `libc++.so.1` to `/usr/lpp/xlC/lib/libc++.a`

The patched `libc++.so.1` (from Patch 1) is injected into the system archive
so `shr2_64.o` can load it.

**Command:**
```sh
cp /tmp/libc++.so.1.patched /tmp/libc++.so.1
ar -X64 -q /usr/lpp/xlC/lib/libc++.a '/tmp/libc++.so.1'
```

**Revert:**
```sh
ar -X64 -d /usr/lpp/xlC/lib/libc++.a 'libc++.so.1'
```

**Final state of `/usr/lpp/xlC/lib/libc++.a`:**
```
shr.o shr_64.o cxxabi.o cxxabi_64.o shr.imp shr_64.imp cxxabi.imp cxxabi_64.imp
shr2_64.o libc++.so.1
```
(original 8 members + 2 added)

---

## Patch 5 — Add `libc++abi.so.1` to `/opt/freeware/lib/libc++abi.a`

`libc++.so.1` imports `__xlcxx_personality_v1` and several C++17 symbols
(`_ZnwmSt11align_val_t`, `_ZdlPvSt11align_val_t`, etc.) from
`libc++abi.a[libc++abi.so.1]`. The AIX 7.2 `libc++abi.a` from the toolbox
(version 16.1.0.7) does not export these symbols. The AIX 7.3 version (16.1.0.10)
does.

**Command:**
```sh
# On AIX 7.3: extract
ar -X64 -x /usr/lpp/xlC/lib/libc++abi.a 'libc++abi.so.1'
# Transfer to AIX 7.2, then:
ar -X64 -q /opt/freeware/lib/libc++abi.a 'libc++abi.so.1'
```

**Revert:**
```sh
ar -X64 -d /opt/freeware/lib/libc++abi.a 'libc++abi.so.1'
```

---

## Patch 6 — Add Dynamic Members to Embedded Library Archives

When `LIBPATH=$EMBEDDED_DESTDIR/lib:/opt/freeware/lib:...` is set (required to
ensure Python's embedded OpenSSL takes precedence over the toolbox's), Rust
tools (`rustc`, `cargo`) resolve their shared library dependencies from the
embedded archives first. The embedded archives are **static** (contain `.o`
object files), while Rust was compiled against **dynamic** archives (containing
`.so.N` shared members). The fix is to add the needed dynamic members from the
toolbox into the embedded archives.

### 6a — `libz.so.1` into embedded `libz.a`

`librustc_driver` needs `libz.a(libz.so.1)`. The embedded `libz.a` (built from
source in stage 01) only has static `.o` members.

```sh
# Extract 64-bit member from toolbox
ar -X64 -x /opt/freeware/lib/libz.a 'libz.so.1'
# Add to embedded archive
ar -X64 -q /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libz.a 'libz.so.1'
```

### 6b — `libssl.so.3` into embedded `libssl.a`

`cargo` needs `libssl.a(libssl.so.3)`. The embedded `libssl.a` has only
`libssl64.so.3` (note the `64` suffix — different member name).

```sh
ar -X64 -x /opt/freeware/lib/libssl.a 'libssl.so.3'
ar -X64 -q /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libssl.a 'libssl.so.3'
```

### 6c — `libcrypto.so.3` into embedded `libcrypto.a`

Same issue as `libssl.so.3`.

```sh
ar -X64 -x /opt/freeware/lib/libcrypto.a 'libcrypto.so.3'
ar -X64 -q /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libcrypto.a 'libcrypto.so.3'
```

---

## Patch 7 — Rust SDK Community License Package

The `rustc.orig` binary checks for a license file at startup. Without the
community license RPM installed, `rustc` prints "IBM Open SDK for Rust on AIX
license not found!" and exits with code 1.

**Package:** `rust1.92-community-license-1.92.0-2.aix7.2.ppc.rpm`  
**Source:** `https://public.dhe.ibm.com/aix/freeSoftware/aixtoolbox/RPMS/ppc-7.2/rust/`

```sh
rpm -Uvh --nodeps rust1.92-community-license-1.92.0-2.aix7.2.ppc.rpm
```

---

## Build Environment Variables for Rust Stages

These must be exported before any `pip install` that triggers a Rust compilation
(stages 05, 06, 07):

```sh
# embedded lib first so Python SSL finds embedded OpenSSL, then freeware for
# libzz.a (strftime_l stub), libunwind.a, and the .so.3 members above
export LIBPATH=$EMBEDDED_DESTDIR/lib:/opt/freeware/lib:${LIBPATH:-}

# /tmp is only 3GB on this VM; cargo's crates.io index needs hundreds of MB
export TMPDIR=$BUILD_DIR/gotmp

# The full git-based crates.io index (~400MB) causes cargo to SIGSEGV on AIX
# 7.2 (crash in libgit2 or JSON processing during large graph resolution).
# The sparse protocol fetches only needed crate metadata — lightweight and stable.
export CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse
```

---

## Pre-built Wheels for cargo-dependent Python Packages

Two Python packages that require Rust compilation fail because `cargo metadata`
SIGSEGV's when resolving their complex dependency graphs on this system (4GB RAM
+ AIX-specific cargo quirk). They are built on AIX 7.3 (where Rust SDK works),
their `.so` binaries verified to have no `strftime_l` or C++ runtime imports,
retagged from `aix_7302_*` or `aix_3_*` to `aix_7202_2015_64`, and placed in
`$BUILD_DIR/aix72-wheels/` for `--find-links` resolution.

| Package | Version | Note |
|---------|---------|------|
| `maturin` | 1.13.3 | Build tool for Rust Python extensions; only imports `libc.a`, `libpthread.a`, `libunwind.a` |
| `jellyfish` | 1.2.1 | String similarity; only imports `libc.a`, `libpthread.a`, `libunwind.a`, `libpython3.13.a`, `libbsd.a` |

Stage 05 uses `--find-links $BUILD_DIR/aix72-wheels` for the maturin install.  
Stage 07 uses `--find-links $BUILD_DIR/aix72-wheels` for all deps installs.

---

## Revert Procedure

To undo all system library changes on the AIX 7.2 VM:

```sh
# Revert libc++.a
ar -X64 -d /usr/lpp/xlC/lib/libc++.a shr2_64.o
ar -X64 -d /usr/lpp/xlC/lib/libc++.a 'libc++.so.1'

# Revert libc++abi.a
ar -X64 -d /opt/freeware/lib/libc++abi.a 'libc++abi.so.1'

# Remove stub library
rm /opt/freeware/lib/libzz.a

# Remove dynamic members from embedded archives
# (or rebuild from stage 01 sentinel by deleting it)
ar -X64 -d /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libz.a 'libz.so.1'
ar -X64 -d /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libssl.a 'libssl.so.3'
ar -X64 -d /opt/dd-build/staging/opt/datadog-agent/embedded/lib/libcrypto.a 'libcrypto.so.3'
```

The embedded dynamic members (patches 6a–6c) are also lost when stage 09
runs (`rm -rf $EMBEDDED_DESTDIR/include`) — they need to be re-applied after
clearing the stage 09 sentinel when re-running stages 05–07.
