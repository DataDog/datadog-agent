# AIX Changelog

<!-- This file tracks changes to the Datadog Agent on AIX.
     It is maintained manually until AIX is officially supported and released
     through the same process as other platforms, at which point it will be
     replaced by the standard release notes mechanism. Each PR that affects
     AIX should add an entry to the current (unreleased) section below. -->

## Unreleased

<!-- Add entries here for changes not yet in a release. -->

- Upgrade embedded pip from 24.0 to 26.0.1 (patches CVE-2026-1703)
- Fix Python entry-point script shebangs in the embedded tree (was pointing to build-host staging path, causing "No such file or directory" when running `pip` or other scripts post-install)
- Include `final_constraints-py3.txt` in the BFF package at `/opt/datadog-agent/final_constraints-py3.txt`

---

> **Note:** `datadog-agent-7.80.0-devel.git.446.66c9b62-18.aix.ppc64.bff` and all older builds
> predate this changelog and do not have entries.
