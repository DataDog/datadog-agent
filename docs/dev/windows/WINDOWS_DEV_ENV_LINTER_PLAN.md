# Plan: Linter Logic for Windows Dev Env

## Context

The Windows dev env already supports running tests via `dda inv test --host windows` and the
`watch` task. The linter path is partially wired — state/output files are already parameterised
by `command_type` (`/tmp/windev_{name}_linter_state.json`, etc.) and `attach_or_run` accepts
`command_type="linter"` — but three gaps remain:

1. `linter.py`'s `go` task has no `host` parameter: there is no `dda inv linter.go --host windows` entry point.
2. `_build_watch_work` always builds a **test** command when packages are modified, even when the fallback command is a linter command.
3. `attach_or_run`'s fresh run path always builds `inv test --build-stdlib --targets=...` when packages are non-empty, ignoring `command_type`.

---

## Files to modify

- `tasks/linter.py`
- `tasks/windows_dev_env.py`

---

## Changes

### 1. `tasks/linter.py` — add `host` parameter and Windows branch

Add `host: str = None` to the `go` task signature (alongside `run_on`).

After `process_input_args` resolves `modules` (and the `if not modules` early-return), add a
Windows branch mirroring the pattern in `gotest.py:382-392`:

```python
if host == "windows":
    print("Running linter on Windows development environment")
    from tasks.windows_dev_env import attach_or_run

    package_list = [
        f"./{os.path.join(m.path, t)}" if not os.path.join(m.path, t).startswith("./") else os.path.join(m.path, t)
        for m in modules
        for t in m.lint_targets
    ]
    exit(attach_or_run(ctx, name="windows-dev-env", command_type="linter", packages=package_list))
```

> `m.lint_targets` is the correct attribute: `GoModule.__post_init__` sets it to `test_targets` when not explicitly configured.

---

### 2. `tasks/windows_dev_env.py` — `_build_watch_work`

Detect `command_type` **before** checking modified packages so the targeted command is built
correctly for both command types:

```python
command_type = "linter" if "linter" in fallback_command else "test"
raw_packages = find_modified_packages(ctx)
if raw_packages:
    packages = _normalize_packages(raw_packages)
    targets = ','.join(f'./{p}' for p in sorted(packages))
    inv_cmd = f"inv linter.go --targets={targets}" if command_type == "linter" \
              else f"inv test --build-stdlib --targets={targets}"
else:
    packages = frozenset()
    inv_cmd = fallback_command
```

---

### 3. `tasks/windows_dev_env.py` — `attach_or_run` fresh run path

The current code always builds a test command when `norm_packages` is non-empty.
Branch on `command_type` instead:

```python
if norm_packages:
    targets = ",".join(f"./{p}" for p in sorted(norm_packages))
    inv_cmd = f"inv linter.go --targets={targets}" if command_type == "linter" \
              else f"inv test --build-stdlib --targets={targets}"
elif command_type == "test":
    inv_cmd = "inv test --build-stdlib"
else:
    inv_cmd = "inv linter.go"
```

---

## What does NOT need to change

| Component | Reason |
|---|---|
| State/output file helpers | Already parameterised by `command_type` |
| `_test_runner_loop` | Already handles both types generically |
| `_normalize_packages` | Works for both |
| `attach_or_run` matching logic | Already compares `command_type` + packages subset |
| `watch` task | Already accepts `--command "inv linter.go"` as the fallback |

---

## Usage after implementation

```bash
# Watch + auto-lint on .go file changes
dda inv windows-dev-env.watch --command "inv linter.go"

# Attach/replay linter result (or fresh run if no watcher)
dda inv linter.go --host windows --only-modified-packages
dda inv linter.go --host windows --targets ./pkg/util/json
```

---

## Verification

1. Start watcher: `dda inv windows-dev-env.watch --command "inv linter.go"`
2. Modify a `.go` file — confirm `/tmp/windev_windows-dev-env_linter_state.json` shows
   `"command": "linter"` with correct packages
3. Run `dda inv linter.go --host windows --only-modified-packages` — should attach or replay
4. Run again immediately — should replay without re-running
5. Confirm test state file (`_test_state.json`) is unaffected
