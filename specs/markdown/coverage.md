# Test Coverage
## Fleet-Installer
### Functional (27 requirements)
:::{dropdown} Automated (0 — 0%)
:icon: check-circle-fill
:color: secondary

:::
:::{dropdown} Partially Manual (0 — 0%)
:icon: gear
:color: secondary

:::
:::{dropdown} Manual (4 — 15%)
:icon: pencil
:color: warning

- [`FUN-001` Install a package from OCI registry](/requirements/FUN-001)
- [`FUN-002` Remove an installed package](/requirements/FUN-002)
- [`FUN-014` Custom OCI registry and authentication](/requirements/FUN-014)
- [`FUN-015` Bootstrap installation](/requirements/FUN-015)
:::
:::{dropdown} No Test Plan (23 — 85%)
:icon: x-circle-fill
:color: danger

- [`FUN-003` Start an experiment upgrade](/requirements/FUN-003)
- [`FUN-004` Promote an experiment to stable](/requirements/FUN-004)
- [`FUN-005` Stop an experiment (rollback)](/requirements/FUN-005)
- [`FUN-006` Query package state](/requirements/FUN-006)
- [`FUN-007` Garbage collect unused versions](/requirements/FUN-007)
- [`FUN-008` Install config experiment](/requirements/FUN-008)
- [`FUN-009` Promote or rollback config experiment](/requirements/FUN-009)
- [`FUN-010` Install and remove extensions](/requirements/FUN-010)
- [`FUN-011` Persist extensions across upgrades](/requirements/FUN-011)
- [`FUN-012` Configure via environment variables](/requirements/FUN-012)
- [`FUN-013` Default package resolution](/requirements/FUN-013)
- [`FUN-016` Remote-triggered operations](/requirements/FUN-016)
- [`FUN-017` Local HTTP API](/requirements/FUN-017)
- [`FUN-018` Setup flavors](/requirements/FUN-018)
- [`FUN-019` Repair damaged installation](/requirements/FUN-019)
- [`FUN-020` Rollback on failed upgrade](/requirements/FUN-020)
- [`FUN-021` Accept MSI configuration parameters](/requirements/FUN-021)
- [`FUN-022` Custom install directories](/requirements/FUN-022)
- [`FUN-023` Major and minor version upgrades](/requirements/FUN-023)
- [`FUN-024` Prevent downgrades](/requirements/FUN-024)
- [`FUN-025` User account management](/requirements/FUN-025)
- [`FUN-026` Change ddagentuser on existing install](/requirements/FUN-026)
- [`FUN-027` Create install_info file](/requirements/FUN-027)
:::
### Non-Functional (5 requirements)
:::{dropdown} Automated (0 — 0%)
:icon: check-circle-fill
:color: secondary

:::
:::{dropdown} Partially Manual (0 — 0%)
:icon: gear
:color: secondary

:::
:::{dropdown} Manual (0 — 0%)
:icon: pencil
:color: secondary

:::
:::{dropdown} No Test Plan (5 — 100%)
:icon: x-circle-fill
:color: danger

- [`NFR-001` Installer signing](/requirements/NFR-001)
- [`NFR-002` Least privilege](/requirements/NFR-002)
- [`NFR-003` Clean state on failure](/requirements/NFR-003)
- [`NFR-004` Atomic repository operations](/requirements/NFR-004)
- [`NFR-005` Telemetry](/requirements/NFR-005)
:::

