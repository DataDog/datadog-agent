# Network Path

Covers `pkg/networkpath/`, `comp/networkpath/`, `pkg/collector/corechecks/networkpath/`,
`comp/core/autodiscovery/providers/networkpath/`, and the system-probe module in
`cmd/system-probe/modules/traceroute*.go`. The traceroute probing itself (TCP SYN/SACK, UDP, ICMP,
raw sockets, the Windows driver path) lives in the external `github.com/DataDog/datadog-traceroute`
module. Change it there, then bump its version in the root `go.mod`.

## Where a traceroute runs

- **Scheduled tests:** the core agent's `network_path` check (`conf.d/network_path.d`, or the
  Autodiscovery provider when `network_path.remote_config.enabled` is set) calls `traceroute/fx-remote`.
  That makes an HTTP request over `system_probe_config.sysprobe_socket` to system-probe's `traceroute`
  module (`/traceroute/{host}`), which calls `pkg/networkpath/traceroute/runner`.
- **Dynamic tests (from CNM connections):** `npcollector` schedules them. With
  `network_config.direct_send: false` it runs in process-agent (connections check) and uses
  `fx-remote`. With direct send (Linux only) it runs inside system-probe and uses `fx-local`, with no
  HTTP hop. NetFlow-sourced tests come from `comp/netflow/flowaggregator`.
- **system-probe `traceroute.enabled`:** if it isn't set, it turns on automatically when dynamic tests
  are enabled. An explicit `false` wins and disables basic dynamic tests.
- A request parameter added to the remote component must also be parsed in
  `cmd/system-probe/modules/traceroute.go` (`parseParams`).

## Unit tests

- **Pass `-tags test`.** Most tests and mocks here are `//go:build test`. Without the tag, packages
  fail with `undefined: logmock.New`, or report `[no test files]` and run nothing:
  `go test -tags test ./comp/networkpath/... ./pkg/networkpath/... ./pkg/collector/corechecks/networkpath/... ./comp/core/autodiscovery/providers/networkpath/...`
- **`pkg/networkpath/payload` is its own Go module.** `./pkg/networkpath/...` skips it, so list it
  separately: `go test -tags test ./pkg/networkpath/payload/`.
- **On a fresh clone or worktree,** run `dda inv schema.codegen` before plain `go test`, otherwise
  `pkg/config/setup` fails with `undefined: generatedsettings.InitCommonBase`. `dda inv` and Bazel
  generate it themselves.
- **Bazel runs the same set,** including `payload`, with the right tags:
  `bazel test //comp/networkpath/... //pkg/networkpath/... //pkg/collector/corechecks/networkpath/... //comp/core/autodiscovery/providers/networkpath/...`
- **The system-probe module tests are `linux || windows`.** On macOS they report
  `[no tests to run]`. On a Linux host or VM:
  `go test -tags test ./cmd/system-probe/modules/ -run 'TestParseParams|TestHandleTraceroute'`.
  These tests and the runner tests don't need root or eBPF.
- `runner_linux.go` (gateway lookup via the network namespace) is compiled only on Linux. Check that
  it compiles from macOS with
  `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -tags test -o /dev/null ./pkg/networkpath/traceroute/runner/`.

## End-to-end tests

Real probing is covered only by E2E in `test/new-e2e/tests/netpath/`
(`network-path-integration` for scheduled tests on Linux and Windows hosts, `dynamic-tests` for
CNM- and NetFlow-driven ones). They provision cloud hosts; see `test/new-e2e/AGENTS.md`, and ask
before running them. In CI, `new-e2e-netpath` and `new-e2e-netpath-windows` always run on `main`,
release branches, and RC tags, and when shared e2e files change (`.on_e2e_main_release_or_rc`). On
other PRs they run automatically only for the paths in `.on_netpath_or_e2e_changes` in
`.gitlab-ci.yml`, and are otherwise left as manual jobs. That list includes
`cmd/system-probe/modules/traceroute.go` but not `traceroute_{linux,windows,darwin}.go` or the root
`go.mod`, so on a PR, a driver change or a `datadog-traceroute` bump doesn't start them; run them by
hand.

## Keeping these instructions current

When you work out something non-obvious that cost real effort and isn't written down here, propose
adding it: a test or build command, a platform-specific failure and its fix, or a debugging step.
Also flag anything you found wrong or stale.

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
