# AIX Changelog

<!-- This file tracks changes to the Datadog Agent on AIX.
     It is maintained manually until AIX is officially supported and released
     through the same process as other platforms, at which point it will be
     replaced by the standard release notes mechanism. Each PR that affects
     AIX should add an entry to the current (unreleased) section below. -->

## Unreleased

<!-- Add entries here for changes not yet in a release. -->

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
