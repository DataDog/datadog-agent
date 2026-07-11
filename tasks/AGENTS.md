# tasks/ — Invoke Tasks Overview

This directory contains Python [Invoke](https://www.pyinvoke.org/) tasks, exposed
via the `dda inv` CLI wrapper. Much developer-facing automation lives here: building,
testing, releasing, CI management, and tooling.

## Directory Layout

```
tasks/
├── *.py                — top-level task modules; each @task is a dda inv subcommand
├── libs/               — shared library code (no @task decorators)
│   ├── common/         — utilities used across many tasks (git, go, color, s3, …)
│   ├── ciproviders/    — GitHub and GitLab API clients
│   ├── pipeline/       — CI pipeline data structures and generation helpers
│   ├── releasing/      — version arithmetic and release JSON helpers
│   ├── testing/        — test result parsing, flake detection
│   └── types/          — shared dataclasses and enums
├── unit_tests/         — pytest tests for task logic (run via dda inv invoke-unit-tests.run)
├── custom_task/        — InvokeLogger: wraps every @task call and logs it to Datadog
└── BUILD.bazel         — Bazel targets for task code that has been migrated
```

## Task Categories

### Build and Test

Tasks that compile agent binaries or run test suites. Each agent binary has its own
module (`agent.py`, `cluster_agent.py`, `dogstatsd.py`, `trace_agent.py`,
`system_probe.py`, `process_agent.py`, `security_agent.py`, …). Cross-cutting build
concerns live in `gotest.py`, `linter.py`, `build_tags.py`, `modules.py`, and
`rtloader.py`.

Key patterns:
- Build tasks call `dda inv <component>.build`. Never shell out to raw `go build`.
- Build tags are computed from `build_tags.py` / `build_tags.bzl`, which is the
  single source of truth. Do not hardcode tag lists in task code.
- `libs/common/go.py` contains pure Go-related helpers (module downloading, etc.)
  that do not depend on `@task`.

### CI / Pipeline

Tasks that interact with GitLab CI, GitHub Actions, or internal CI systems.
Primary files: `pipeline.py`, `gitlab_helpers.py`, `github_tasks.py`, `notify.py`,
`agent_ci_api.py`, `junit_tasks.py`.

Supporting library code:
- `libs/ciproviders/gitlab_api.py` — GitLab REST client helpers
- `libs/ciproviders/github_api.py` — GitHub REST client helpers
- `libs/pipeline/` — pipeline data structures, failure classification, job generation

These tasks are typically CI-only and depend on GitLab/GitHub tokens available in CI
environment variables.

### Everything Else

Release, packaging, tooling, and developer-experience tasks. Examples:
`release.py`, `omnibus.py`, `package.py`, `licenses.py`, `notes.py`,
`components.py`, `go_deps.py`, `go.py`, `modules.py`, `renovate.py`,
`new_e2e_tests.py`, `ebpf.py`.

## Incremental Refactoring: Making Task Logic Bazel-Callable

We are incrementally splitting invoke-coupled code from pure logic so that the
logic can be used as Bazel `py_binary` / `py_library` targets — without requiring
the invoke runtime.

**RFC:** https://docs.google.com/document/d/1TFQHjDzz0a89bddjC5rsNmB8PwKLmbN13ZTihEg0Wl0/edit?tab=t.0#heading=h.ka2x7m2anufz

### The Goal

A Bazel action (e.g. `run_binary`, `py_binary`) cannot depend on invoke. Logic
that needs to run as both a `dda inv` task *and* a Bazel action must be separated
into a standalone Python file with no invoke imports. The `@task` wrapper stays in
the top-level task file and calls into the pure library.

### Anatomy of a Migrated Task

Before (everything in one place):

```python
# tasks/foo.py
from invoke import task
from invoke.context import Context

@task
def compute_thing(ctx: Context, arg: str) -> None:
    result = _do_work(arg)      # private helper — not reachable from Bazel
    ctx.run(f"echo {result}")
```

After (logic extracted):

```python
# tasks/libs/common/foo.py  (no invoke import)
def compute_thing(arg: str) -> str:
    ...                          # pure logic, importable by Bazel

# tasks/foo.py
from invoke import task
from invoke.context import Context
from tasks.libs.common.foo import compute_thing

@task
def compute_thing_task(ctx: Context, arg: str) -> None:
    result = compute_thing(arg)
    ctx.run(f"echo {result}")
```

Then add a Bazel target in `tasks/BUILD.bazel` (or `tasks/libs/common/BUILD.bazel`):

```python
py_binary(
    name = "compute_thing",
    srcs = ["libs/common/foo.py"],
    main = "libs/common/foo.py",
)
```

### Existing Examples

- `tasks/libs/common/get_payload_version.py` — reads `go.mod`, emits a Bazel
  Starlark constant; zero invoke imports; exposed as `//tasks:get_payload_version_tool`.

### Rules of Thumb

1. **`libs/` is invoke-free by convention** — avoid adding `from invoke` imports to
   files under `tasks/libs/`. If a helper truly needs `ctx`, keep it in a top-level
   task file or accept `ctx` as an optional parameter with a default of `None`.
2. **Entry-point files may be dual-use** — a file can be both a `@task` module and
   a standalone script if it uses `if __name__ == "__main__"` guards and avoids
   importing invoke at module level.
3. **`BUILD.bazel` targets live close to the code** — prefer adding targets in the
   sub-directory's `BUILD.bazel` rather than the root `tasks/BUILD.bazel`, unless
   the target is repo-wide infrastructure.

### Discovered Migration Idioms

> **Instructions for AI agents:** when working on a migration in this directory,
> if you discover a non-obvious pattern, workaround, or pitfall not covered above,
> add it to this section before finishing the session. Keep entries concise
> (2-4 lines each) and include *why* the idiom is needed.

<!-- Add new idioms below this line -->
