# Testing Network Device Monitoring changes

Applies to `pkg/collector/corechecks/snmp/`, `pkg/networkdevice/`, `pkg/networkdevices/`, `pkg/snmp/`,
`comp/ndm*/`, `comp/snmptraps/`, `comp/snmpscan*/`, `comp/netflow/`, `pkg/networkconfigmanagement/`,
`comp/networkconfigmanagement/`, and `pkg/collector/corechecks/networkconfigmanagement/`.

## Unit tests

All of these run on macOS, Linux, and Windows; none need a Linux kernel.

- **Preferred:** Bazel, which applies the `test` tag, runs the `ncm` variants, and crosses nested
  `go.mod` boundaries. The whole NDM scope:

  ```sh
  bazel test //pkg/collector/corechecks/snmp/... //pkg/networkdevice/... //pkg/networkdevices/... \
    //pkg/snmp/... //comp/ndm/... //comp/ndmconnectivitycheck/... //comp/ndmdiscovery/... \
    //comp/ndmtmp/... //comp/snmptraps/... //comp/snmpscan/... //comp/snmpscanmanager/... \
    //comp/netflow/... //pkg/networkconfigmanagement/... //comp/networkconfigmanagement/... \
    //pkg/collector/corechecks/networkconfigmanagement/...
  ```

  `dda inv test --targets=./pkg/networkdevice,...` runs the same recursive Bazel patterns.
- **Plain `go test` needs `-tags test`.** Without it, mocks and helpers (`pkg/aggregator/mocksender`,
  `comp/*/mock`, `fake_session.go`) are excluded (`build constraints exclude all Go files`), and tests that
  set config panic with `can only call SetInTest from test code`.
- **NCM needs `-tags test,ncm`.** The real check and component are behind `ncm`; without it you build
  the stub, `pkg/collector/corechecks/networkconfigmanagement` reports `[no test files]`, and about half
  of `comp/networkconfigmanagement/impl`'s tests silently don't run.
- **Plain `go test` skips nested modules.** `go test ./pkg/networkdevice/...` doesn't include
  `pkg/networkdevice/profile` (its own `go.mod`); test it explicitly. Bazel includes it.
- **Fresh clone or worktree:** run any `dda inv` task (for example `dda inv schema.codegen`) before
  plain `go test`, or `pkg/config/setup` fails with `undefined: generatedsettings.InitCommonBase`.

## Profiles

- **SNMP default profiles are not in this repo.** The check reads them at runtime from
  `<confd_path>/snmp.d/default_profiles/` (user profiles from `snmp.d/profiles/`). Packaging copies them
  from integrations-core's `snmp/datadog_checks/snmp/data/` at the `INTEGRATIONS_CORE_VERSION` commit in
  `release.json`. Change a shipped profile in integrations-core, not here.
- **SNMP test profiles are shared fixtures.** `pkg/collector/corechecks/snmp/internal/test/` is used by
  tests in several packages, and `FixtureProfileDefinitionMap()` in `internal/profile/testing_utils.go`
  hand-copies `conf.d/snmp.d/profiles/f5-big-ip.yaml`. Changing one value in that YAML fails tests in
  `snmp`, `checkconfig`, `devicecheck`, and `profile`. Add a new `*.d/` fixture directory instead of
  editing an existing one; if you must edit, update the Go copy and run the whole `snmp/...` tree.
- **NCM fixtures are embedded.** `pkg/networkconfigmanagement/profile/fixtures/` is read through
  `//go:embed`, and Bazel only embeds the files listed in `embedsrcs`. After adding a fixture, run
  `bazel run //:gazelle -- pkg/networkconfigmanagement/profile`: `go test` passes without it, but the
  Bazel test fails with `file does not exist`. NCM's default profiles are Go code in
  `default_profiles.go`.
- **Profile schema is generated.** After changing `pkg/networkdevice/profile/profiledefinition` types,
  run `go generate ./...` in `profiledefinition/schema_cmd` and commit `schema/profile_rc_schema.json`;
  `TestGenerateJSONSchemaIsInSync` fails otherwise.

## E2E

- `test/new-e2e/tests/ndm/snmp` (agent plus snmpsim, replaying the `compose/data/*.snmprec` recordings) and
  `test/new-e2e/tests/ndm/netflow` provision cloud VMs; they don't run locally. Typecheck them with
  `go vet ./tests/ndm/...` from `test/new-e2e`, and see `test/new-e2e/AGENTS.md` to run them.
- The SNMP and NetFlow e2e jobs always run on `main`, release branches, and RC tags, and when shared
  e2e files such as `test/new-e2e/go.mod` change (`.on_e2e_main_release_or_rc` in `.gitlab-ci.yml`).
  On other PRs, the SNMP job runs automatically only when `pkg/collector/corechecks/snmp/` or
  `test/new-e2e/tests/ndm/snmp/` changes (`.on_ndm_snmp_or_e2e_changes` in
  `.gitlab/test/e2e/e2e_ndm.yml`), so a change limited to `pkg/snmp/`, `pkg/networkdevice/`, or
  `comp/snmptraps/` leaves it as a manual job. Start it by hand before trusting such a change.

## Pull requests

Label with `team/network-device-monitoring-core` or `team/ndm-integrations`, matching the owner in
`.github/CODEOWNERS`.

## Keeping these instructions current

When you work out something non-obvious that cost real effort and isn't written down here, propose
adding it: a test or build command, a tag- or platform-specific failure and its fix, or a debugging
step. Also flag anything you found wrong or stale.

- **Verify it first.** Check the command, flag, or behavior against the code or tooling. If you
  can't verify it, don't add it; tell the user what's unconfirmed.
- **Keep it short and imperative,** in the style of the surrounding section. No narrative, dates,
  or issue numbers.
- **Keep it generic.** This is a public repository: no internal URLs, hostnames, chat channels,
  internal repository names, or customer names. Personal setup (accounts, machine paths, local
  workarounds) goes in your local, uncommitted instructions instead.
- **Ship it as its own pull request.** Don't fold it into the change you're working on. Tell the
  user what you'd add and where, and suggest a small separate pull request so it's reviewed on its
  own.
