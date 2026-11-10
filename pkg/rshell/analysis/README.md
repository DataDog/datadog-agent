# analysis

Static enforcement of symbol-level import restrictions for the rshell interpreter.

## Purpose

rshell is a sandboxed shell. Any Go package it imports is a potential escape vector. This package maintains **explicit allowlists** of every `importpath.Symbol` that each subsystem is permitted to use, and enforces those lists via Go AST analysis run as tests.

If a symbol is not on the list, the code cannot compile it through CI — adding new capabilities requires a deliberate, reviewed allowlist entry.

## Permanently Banned Packages

Some packages may never be imported regardless of the symbol, declared in `symbols_common.go`:

## Allowlists

Each subsystem has its own allowlist file:

| File | Governs |
|------|---------|
| `symbols_builtins.go` | `builtins/` — builtin command implementations |
| `symbols_interp.go` | `interp/` — interpreter core |
| `symbols_allowedpaths.go` | `allowedpaths/` — filesystem sandbox |
| `symbols_fsstat.go` | `allowedpaths/internal/fsstat/` — sandboxed filesystem-stat platform backends |
| `symbols_internal.go` | `builtins/internal/` — shared internal helpers |

### Two-layer system for builtins

Builtins use two complementary lists:

- **`builtinAllowedSymbols`** — global ceiling: every symbol any builtin may use.
- **`builtinPerCommandSymbols`** — per-command sublists: each builtin directory (`cat/`, `grep/`, …) declares only the symbols it actually needs.

Every symbol in a per-command list must be present in the global ceiling. Every symbol in the global ceiling must appear in at least one per-command list. This keeps each builtin's surface area minimal and auditable in isolation.

The same two-layer pattern applies to `builtins/internal/` via `internalAllowedSymbols` and `internalPerPackageSymbols`.

## Safety Legend

Each allowlist entry carries an inline comment prefixed with a safety emoji:

| Emoji | Meaning | Examples |
|-------|---------|---------|
| 🟢 | **Pure** — no side effects (pure functions, constants, types, interfaces) | `strings.Split`, `fmt.Sprintf`, `io.Reader`, AST types |
| 🟠 | **Read-only I/O** — reads from filesystem, OS state, or kernel; or delegates writes to an `io.Writer` | `os.Open`, `os.ReadFile`, `time.Now`, `net.Interfaces`, `syscall.Getsid` |
| 🔴 | **Privileged** — network I/O, unsafe memory, or native code loading | `net.DefaultResolver`, `pro-bing.NewPinger`, `unsafe.Pointer`, `syscall.MustLoadDLL` |

## Enforcement

The tests in this package use Go's `go/parser` and `go/ast` to walk source files and verify:

1. No permanently banned package is imported.
2. Every imported package is in the allowlist.
3. Every `pkg.Symbol` reference is explicitly listed.
4. Every symbol in an allowlist is actually used (no dead entries).
5. Every builtin subdirectory has a per-command entry.

Verification tests additionally inject banned imports or unlisted symbols into a temporary copy of the repo and assert the checker catches them.

## Structural Rules

In addition to symbol-level allowlist checking, the package enforces **structural rules** — code patterns that must (or must not) appear together in the same function scope. These are checked by `checkFileScannerBuffer` and `checkFileOpenFileClose` in `structural.go` and are applied automatically to every file that passes through `checkAllowedSymbols`.

Both rules are also exposed as standalone `go/analysis` analyzers (`ScannerBufferAnalyzer`, `OpenFileCloseAnalyzer`) that can be registered with `go vet` or gopls.

### Rule 1 — `bufio.NewScanner` must call `.Buffer()`

**Why:** `bufio.Scanner` has a fixed default buffer of 64 KiB. Any line longer than that causes `Scanner.Scan()` to return `false` and `Scanner.Err()` to return `bufio.ErrTooLong`. In a shell that must handle arbitrary user input this is a reliability and DoS risk — a single long line silently truncates or aborts processing.

**What is checked:** Every variable assigned from `bufio.NewScanner(...)` must have `.Buffer(...)` called on it within the same function body. Nested function literals (`func() { ... }`) are treated as independent scopes.

**Compliant:**
```go
sc := bufio.NewScanner(r)
sc.Buffer(make([]byte, 4096), maxLineBytes)
for sc.Scan() { ... }
```

**Violation:**
```go
sc := bufio.NewScanner(r)  // flagged: no sc.Buffer() call
for sc.Scan() { ... }
```

### Rule 2 — `callCtx.OpenFile` results must be closed

**Why:** Every open file descriptor consumes a kernel resource. Over repeated script executions, unclosed handles exhaust the process file-descriptor limit and cause all subsequent I/O to fail.

**What is checked:** Every variable assigned from a `.OpenFile(...)` call (any receiver — the check matches the method name, not the receiver type) must have `.Close()` called on it — directly or via `defer` — within the same function body. The checker also tracks **hand-off** assignments: if `f` is reassigned to `rc` and `rc.Close()` is called, `f` is considered closed.

**Compliant — direct close:**
```go
f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
if err != nil { return err }
defer f.Close()
```

**Compliant — hand-off pattern:**
```go
f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
if err != nil { return err }
rc = f          // hand off to rc
defer rc.Close() // closes f transitively
```

**Compliant — return ownership transfer:**
```go
func openHelper(callCtx cc) (io.ReadCloser, error) {
    f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
    if err != nil { return nil, err }
    return f, nil  // caller is responsible for closing
}
```

**Violation:**
```go
f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
if err != nil { return err }
_ = f  // flagged: f is never closed
```

## Adding a New Symbol

1. Add a line to the appropriate allowlist (and the per-command sublist if it's a builtin).
2. Prefix the comment with the correct safety emoji.
3. Run `dda inv test --module=pkg/rshell --targets=analysis` from the Agent root to verify the entry is valid and used.
