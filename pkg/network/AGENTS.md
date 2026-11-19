# pkg/network

Before testing a change here or claiming it works, read `pkg/ebpf/AGENTS.md` (testing system-probe and
eBPF changes).

- **Classification tests can pass on the wrong connection.** `waitForConnectionsWithProtocol` matches
  any connection between the given addresses, so cases that share a server port can pass on an earlier
  case's connection. Give each case its own port, and show a new test fails against a deliberately
  broken implementation before trusting it.

## Keeping these instructions current

When you work out something non-obvious that cost real effort and isn't written down here or in
`pkg/ebpf/AGENTS.md`, propose adding it: a test or build command, a kernel- or build-mode-specific
failure and its fix, or a debugging step. Also flag anything you found wrong or stale.

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
