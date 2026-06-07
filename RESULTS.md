# Nix Dev Shell — CI Target Verification Results

Branch: `nick/nix-investigation`  
Shell: `devShells.default`  
Host: aarch64 Linux (workspace)  
Date: 2026-06-07

## Command results

| Command | Result | Notes (failures only) |
|---|---|---|
| `dda inv system-probe.object-files` | PASS | |
| `dda inv system-probe.build-dyninst-test-programs` | PASS | |
| `dda inv agent.build` | PASS | |
| `dda inv linter.go --build system-probe-unit-tests --cpus 4 --targets ./pkg` | PASS | |
| `dda inv linter.go --targets=./pkg/security/tests --cpus 4 --build-tags="functionaltests stresstests trivy containerd linux_bpf ebpf_bindata"` | PASS | |
| `dda inv security-agent.run-ebpf-unit-tests --verbose` | PASS | |
| `dda inv system-probe.test --packages='pkg/dyninst/irgen pkg/dyninst/symdb' --extra-arguments=--short --skip-object-files` | PASS | Was failing: `sudo` reset PATH, dropping Nix `go`. Fixed in `tasks/system_probe.py` (commit `5341db19`). |
| `dda inv test --flavor dogstatsd --race --profile --rerun-fails=2 --cpus 4` | PASS | |
| `dda inv test --flavor heroku --race --profile --rerun-fails=2 --cpus 4` | FAIL | 3 env-specific tests (see below) — not a Nix issue |
| `dda inv test --flavor iot --race --profile --rerun-fails=2 --cpus 4` | FAIL | Same 3 env-specific tests — not a Nix issue |

### Failing tests (heroku + iot flavors)

| Test | Root cause | Category |
|---|---|---|
| `pkg/process/procutil :: TestBootTimeRefresh` | Mock `/proc/stat` file not created (flaky under `-race`) | Flaky |
| `comp/host-profiler/symboluploader :: TestSymbolUpload/Upload_if_symtab` | 3-minute timeout — needs internal Datadog symbol upload endpoint | Incompatible host |
| `pkg/fleet/installer/packages/apminject :: TestVerifySharedLib_BuggyLibrary` | Nil pointer dereference — APM shared library not present on host | Incompatible host |

## CI job mapping

| CI job | Commands | Result |
|---|---|---|
| `tests_ebpf_arm64` | 6 commands (same list as above) | 6/6 PASS (sudo/PATH fixed) |
| `tests_ebpf_x64` | Same 6 commands | Not run — requires x86_64 host |
| `tests_flavor_dogstatsd_linux-x64` | `agent.build` + `test --flavor dogstatsd` | PASS |
| `tests_flavor_heroku_linux-x64` | `agent.build` + `test --flavor heroku` | FAIL (3 env-specific tests) |
| `tests_flavor_iot_linux-x64` | `agent.build` + `test --flavor iot` | FAIL (same 3 tests) |

---

## Release toolchain verification (`devShells.release`)

Shell: `devShells.release`  
Date: 2026-06-07

`bash tasks/nix-verify.sh --suite=release` inside `nix develop .#release`:

| Check | Result | Notes |
|---|---|---|
| x86_64 cross-gcc present | PASS | `x86_64-unknown-linux-gnu-gcc (GCC) 11.4.0` |
| aarch64 cross-gcc present | PASS | `aarch64-unknown-linux-gnu-gcc (GCC) 11.4.0` |
| agent.build with Nix release toolchain | PASS | ~89s; cmake uses cross-compiler via `DD_CMAKE_TOOLCHAIN` |
| glibc floor <= 2.23 (aarch64 release target) | PASS | Max ref: `GLIBC_2.17` |
| EMBEDDED_PYTHON set to 3.12.6 | PASS | `/nix/store/…-python3-3.12.6` |
| embedded Python has libpython3.12.so (shared) | PASS | |

Cross-toolchain store path: `/nix/store/i8m6im25pdn2m9fimp8j5yda1v9n6pcn-dd-cross-toolchains-1.26.0`  
Built via: ctng 1.26.0, glibc 2.23 sysroot (aarch64) / 2.17 sysroot (x86_64)

### Fixes required to reach 6/6 (commit `5341db19`)

| Problem | Fix |
|---|---|
| cmake defaulted to host GCC 15.2 → rtloader had `dlopen@GLIBC_2.34` | `nix-verify.sh` generates a cmake toolchain file (`DD_CMAKE_TOOLCHAIN`) pointing at the ctng cross-compiler; cleans `rtloader/build` before each run |
| `tasks/agent.py` set `embedded_path = EMBEDDED_PYTHON` (read-only Nix store) → `make install` failed | Only `python_home_3` is set from `EMBEDDED_PYTHON`; `embedded_path` resolves to writable `dev/` |
| `2>&1 \| tail -5` in the agent.build check masked exit code (tail always exits 0) | Changed to `bash -eo pipefail -c` |
| `from packaging.version import Version` not available in Nix Python | Replaced with stdlib tuple comparison |

### Known limitation

The glibc floor check inspects the **agent ELF only**. Bundled `libpython3.12.so`
is built by Nix against glibc 2.42 and requires glibc 2.34+ at runtime. A fully
glibc-2.23-runnable artifact requires cross-compiling Python from source against
the old sysroot (not yet done — see `nix-full.md` TBD-5a and the embedded Python
plan).
