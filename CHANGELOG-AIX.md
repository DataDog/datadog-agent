# AIX Changelog

<!-- This file tracks changes to the Datadog Agent on AIX.
     It is maintained manually until AIX is officially supported and released
     through the same process as other platforms, at which point it will be
     replaced by the standard release notes mechanism. Each PR that affects
     AIX should add an entry to the current (unreleased) section below. -->

## Unreleased

<!-- Add entries here for changes not yet in a release. -->

- Set `GOMEMLIMIT=2GiB` on build hosts with less than 6 GiB RAM to prevent Go compiler swap thrash (grouped with the existing `-p=1` flag for the same reason)

---

> **Note:** `datadog-agent-7.80.0-devel.git.446.66c9b62-18.aix.ppc64.bff` and all older builds
> predate this changelog and do not have entries.
