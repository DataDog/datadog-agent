# AIX Changelog

<!-- This file tracks changes to the Datadog Agent on AIX.
     It is maintained manually until AIX is officially supported and released
     through the same process as other platforms, at which point it will be
     replaced by the standard release notes mechanism. Each PR that affects
     AIX should add an entry to the current (unreleased) section below. -->

## Unreleased

<!-- Add entries here for changes not yet in a release. -->
- Fix `ibm_mq` check queue discovery on AIX: patch `pymqi`'s `MQENC_NATIVE` constant from `0x222` (little-endian) to `0x111` (big-endian) after install. The constant is generated from Linux headers and caused `MQRCCF_CFH_LENGTH_ERROR` when the check sent PCF commands to a local MQ queue manager.
- Build scripts: remove all hardcoded `/opt/datadog-agent` source-tree references — `AGENT_SRC` is now auto-resolved by walking up from the script directory to the nearest `.git` ancestor, so the agent source can live at any path on the build host
- Remove the obsolete packaged integration constraints artifact from the AIX BFF package
- Remove `sharedlibrarycheck` from the AIX agent build (the shared-library check loader was included but not validated on AIX)
- The embedded `python3.13` binary and all Python extension modules now have the correct install-time library search path baked into their XCOFF loader section. Previously, the staging path was baked in, causing `libpython3.13.so could not be loaded` when running pip or python directly (without `LIBPATH` set). Operators can now run `pip install` without setting `LIBPATH` first.

---

## 7.81.0-devel.git.349.9b5ddbe-1 (2026-06-01)

- Remove static libraries (`libz.a`, `libbz2.a`) from the installed package to avoid conflicts when installing Python C extensions with pip
- Agent and trace-agent wrappers now append the caller's `LIBPATH` to the agent's own library search path, so operator-set paths (e.g. custom driver directories) are visible to the agent at runtime
- `pip install` for C extensions (e.g. `ibm_db` for the DB2 check) no longer requires GCC 8 specifically; the embedded Python now records the compiler as `gcc` so any GCC version in the customer's PATH is used
- Agent and trace-agent wrappers include the `ibm_db` clidriver path (`embedded/lib/python3.13/site-packages/clidriver/lib`) in LIBPATH so the `ibm_db2` check can load `libdb2` at runtime after `pip install ibm_db`
- Bundle `pymqi` in the package so the `ibm_mq` and `ibm_ace` checks work out of the box (no manual `pip install` required); IBM MQ Client 9.1+ must be installed on the target host at runtime
- Go checks: bundle `conf.yaml.example` and `conf.yaml.default` from integrations-core for all Go checks that have them, supplementing the agent-repo config (agent-repo takes precedence on filename conflicts)
- Keep Python headers (`embedded/include/`) in the package so users can build C extension packages (e.g. `ibm_db` for the DB2 check) against the embedded Python, matching Linux/macOS omnibus behaviour
- Include missing Go check configurations in the package: `cisco_sdwan`, `snmp`, `cloud_hostinfo`, `discovery`, `telemetry`, `versa` — these checks are compiled into the agent binary but their config files were missing from `AIX_CORECHECKS`
- Fix SQLite build: link with `-lm` when `SQLITE_ENABLE_MATH_FUNCTIONS` is enabled
- Fix Go agent/trace-agent build: unset `OBJECT_MODE` before invoking the Go external linker in stage 04 (`OBJECT_MODE=64` and `AIX_OBJECT_MODE=64` exported simultaneously cause the linker to pick up the 32-bit `crt0.o`)
- Include `final_constraints-py3.txt` in the BFF package at `/opt/datadog-agent/final_constraints-py3.txt`
- Fix Python entry-point script shebangs in the embedded tree (was pointing to build-host staging path, causing "No such file or directory" when running `pip` or other scripts post-install)
- Upgrade embedded pip from 24.0 to 26.1 (patches CVE-2026-1703, CVE-2026-6357)
- Set `GOMEMLIMIT=2GiB` on build hosts with less than 6 GiB RAM to prevent Go compiler swap thrash (grouped with the existing `-p=1` flag for the same reason)

---

> **Note:** `datadog-agent-7.80.0-devel.git.446.66c9b62-18.aix.ppc64.bff` and all older builds
> predate this changelog and do not have entries.
