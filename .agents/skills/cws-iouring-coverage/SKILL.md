---
name: cws-iouring-coverage
description: Audit CWS (runtime-security) io_uring functional-test coverage and add a functional test for any io_uring opcode whose operation CWS observes but that is not exercised through io_uring. Test-driven — coverage is judged by tests, never by reading eBPF/hook internals. Use when auditing io_uring test coverage or after new IORING_OP_ opcodes appear.
---

# CWS io_uring test coverage audit

For every io_uring operation CWS observes, there must be a functional test that issues the
operation *through io_uring* and asserts the event fires. This skill finds the gaps and fills
them.

## Principle: tests are the only arbiter

Coverage is decided by tests, nothing else. **Do not read eBPF programs or C hooks** (anything
under `pkg/security/ebpf/`) to judge coverage — implementation is out of scope and misleads.
The only question is: *does an io_uring test exist for this operation, and does it pass?*

The one io_uring-specific fact (it's a test assertion, not implementation): io_uring completes
asynchronously, so **every io_uring test must assert `event.async == true`**.

## The audit: build three lists, then intersect

**List A — io_uring opcodes.** `WebFetch https://man7.org/linux/man-pages/man2/io_uring_enter.2.html`
and list every `IORING_OP_*` with the syscall it performs (e.g. `IORING_OP_SOCKET` → `socket(2)`).
Offline fallback: the iouring-go fork is `replace`d in `go.mod` — import path stays
`github.com/iceber/iouring-go`, but source on disk is under
`$(go env GOMODCACHE)/github.com/lebauce/iouring-go@*/syscall/types.go`.

**List B — tested syscalls.** Read `pkg/security/tests/`. A syscall is *tested* when a test
issues it **as the triggering action** (inside the `func() error {…}` passed to
`WaitSignal`/`runSyscallTester`) **and asserts an event** — not when it only appears as setup.
Build the list from test bodies, not event names: the asserted event may be named differently.

**List C — tested io_uring opcodes.** `grep -rn 'iouring\.\|io_uring' pkg/security/tests/*.go`.
Mostly `t.Run("io_uring", …)` subtests plus a few standalone funcs.

**Conclude, per opcode in A:**
- syscall ∉ B → **out of scope** (CWS doesn't observe it; no io_uring test expected).
- syscall ∈ B → **in scope**; then opcode ∈ C → **tested**, else → **gap**.

Write a test for every gap.

## Writing the io_uring test

Model on an existing subtest (`open_test.go` → `t.Run("io_uring", …)`). For each gap:

1. **Rule scope.** The test process is `testsuite`. Many rules are scoped to
   `process.file.name == "syscall_tester"`; reuse the parent rule only if it already admits
   `testsuite` (e.g. `in [ "syscall_tester", "testsuite" ]`), otherwise add a new rule scoped to
   `process.file.name == "testsuite"`. Check the parent's `ruleDefs` first.
2. **Submit.** `iour, err := iouring.New(1)`; submit the op; read the result; in the validation
   callback assert event type, key fields, and `event.async == true`.
3. **Kernel gate, not errno skip.** Gate "kernel too old for this opcode" deterministically at
   the top of the subtest:
   `checkKernelCompatibility(t, "io_uring <op> needs Linux X.Y", func(kv *kernel.Version) bool { return kv.Code < kernel.VersionCode(X, Y, 0) })`.
   Don't skip on a negative errno — a malformed raw SQE returns one too, so skipping would hide
   the gap behind a green test. On a supported kernel, treat an unexpected negative result as a
   **failure** (`return fmt.Errorf(...)`). (A library prep helper can't be malformed, so an
   errno skip there is harmless.)
4. **ebpfless.** Only matters if the parent test is in the `available` list (`~`-prefixed
   entries) in `pkg/security/tests/main_linux.go` — those prefix-match subtests and pull yours
   into the ebpfless run, where io_uring is unsupported. If so, add an `exclude` entry. The
   match is exact on the full `t.Name()`, so prefer a flat sibling name
   (`TestOpen/io_uring_ftruncate`, not a nested `TestOpen/io_uring/ftruncate`).

**No prep helper in the fork?** The fork wraps only a subset. For other opcodes build a raw SQE
with a custom `iouring.PrepRequest` using the helpers in `pkg/security/tests/iouring_test.go`
(extend them as needed). This is the one place you may touch an opcode's low-level shape.

## Verify

- `gofmt -l <your files>` (no output = OK).
- Run the test via the harness:
  `dda inv security-agent.functional-tests --skip-linters --testflags="-test.run <YourTest>"`.
  Green = covered, red = the gap is real.

## Report

Report the per-opcode classification (out of scope / tested / gap) and the test files
added or changed.
