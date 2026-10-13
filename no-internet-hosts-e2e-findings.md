# No-Internet Host E2E Tests — Findings

**Branch:** `kfairise/no-internet-hosts-e2e`
**PR:** https://github.com/DataDog/datadog-agent/pull/58058 (draft, experimental — do not merge)
**Pipeline:** GitLab #144264658
**Status:** IN PROGRESS — updated as failures arrive

## Context

The E2E framework's AWS host provisioner (`awshost.Provisioner`) now prepends a
`WithoutInternetAccess()` default: the VM's default security groups are replaced with
`no-internet-access` groups. Tests that need internet must explicitly opt back in with
`ec2.WithInternetAccess()`.

This branch removes **every explicit `WithInternetAccess()` opt-in from host-based new-e2e
tests** (67 files, 78 call sites) so that they run against the no-internet default. The goal
is not to make them pass, but to inventory:

1. which tests actually depend on internet access,
2. what exactly each one needs from the network, and
3. what would be required to make each pass without internet access.

## Scope

All `ec2.WithInternetAccess()` / `scenec2.WithInternetAccess()` occurrences under
`test/new-e2e/` (host/EC2-VM-based tests) were removed:

| Suite | Files | Notes |
|---|---|---|
| `tests/agent-configuration` | 18 | windows + linux + aws variants |
| `tests/agent-subcommands` | 22 | windows + linux |
| `tests/agent-runtimes` | 12 | incl. infra-mode, procmgr, rtloader |
| `tests/agent-platform` | 9 | install-script, ddot, upgrade, package-signing, macos |
| `tests/installer` | 3 | unix packages, scripts, fapolicyd |
| `tests/fleet` | 3 | suite.go (all platforms), config_macos |
| `tests/cws`, `tests/cspm`, `tests/discovery` | 6 | |
| `tests/ndm/snmp`, `tests/netpath`, `tests/remote-config`, `tests/apm`, `tests/data-security`, `tests/privateactionrunner`, `tests/agent-log-pipelines` | 8 | |
| `examples/` | 2 | host_nopulumi, vm_win |

**Not in scope (left untouched):** the framework's own defaults and helpers
(`test/e2e-framework/...`), the Kubernetes scenarios (EKS / kind / kubeadm — no explicit
opt-ins existed under `test/new-e2e` anyway), and `ec2.ParamsFromEnvironment`'s
append of `WithInternetAccess()` (config-driven path, not a test).

### Known caveats discovered during the removal

- **`tests/data-security/postgres_provisioner.go`** called `ec2.NewVM(..., ec2.WithInternetAccess())`
  directly — **not** through `awshost.Provisioner`. `NewVM` has **no no-internet default**
  (`withoutInternetAccess` defaults to `false`), so removing the option there is a no-op:
  the VM keeps internet. To truly test it, `WithoutInternetAccess()` would have to be added.
- Several tests had in-code comments explaining the internet dependency, e.g.
  `tests/discovery/linux_test.go`: *“provision.sh installs packages from pip, npm and gem”*.
- The macOS fleet config job (`new-e2e-fleet-config-macos`) is manual/nightly-only on
  branch pipelines; its failures won't show up in this run.

## CI coverage of this run

Pipeline jobs relevant to host tests (305 `new-e2e-*` jobs queued in total, incl. k8s ones
not affected by this change). Host-based jobs affected by the removal:

- linux: `agent-configuration`, `agent-subcommands`, `agent-runtimes`, `agent-platform-install-script-*`,
  `agent-platform-ddot-*`, `package-signing-*`, `cspm`, `cws`, `discovery`, `data-security`,
  `ndm-snmp`, `remote-config`, `apm`, `fleet-config` (linux group), `netpath`
- windows: `*-windows` variants (`agent-configuration`, `agent-subcommands`, `agent-runtimes`,
  `cws-windows`, `netpath-windows`, `privateactionrunner-windows`, `fleet-config` windows groups)
- macOS: `agent-platform-install-script-macos-x64` (queued; macOS fleet config is manual)

## Findings per failure

_Filled in as the pipeline progresses. For each failing job: test, root cause, what internet
access it needed, and what would make it pass without internet._

<!-- FINDINGS -->

## Summary

(to be completed at the end)
