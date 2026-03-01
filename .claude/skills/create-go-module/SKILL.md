---
name: create-go-module
description: Create a new Go module in the agent repository
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[module-path]"
---

Create a new Go module in the datadog-agent multi-module repository. This repo has 100+ Go modules managed via `modules.yml` and inter-module `replace` directives.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the path, skip that question.

1. **Module path**: The path relative to the repo root (e.g. `comp/core/mything/def`, `pkg/util/mypkg`). This becomes `github.com/DataDog/datadog-agent/<path>`.

2. **Does the directory already contain Go code?**: If yes, the tool will also update dependent modules. If no, it just creates the `go.mod` and registers the module.

### Step 2: Ensure clean git state

The `create-module` task requires all changes to be committed. Check with `git status` and warn the user if there are uncommitted changes.

### Step 3: Run the create-module task

```bash
dda inv create-module --path=<module-path>
```

This task automatically:
- Creates `go.mod` with the correct module path and Go version
- Adds `replace` directives for inter-module dependencies
- Registers the module in `modules.yml` as `independent: true`
- Runs `tidy` across all modules
- Runs linters (`internal-deps-checker`, `check-mod-tidy`, `check-go-mod-replaces`)

### Step 4: Adjust modules.yml if needed

Read `modules.yml` and review the new entry. The task sets `independent: true` by default. Adjust if needed:

| Field | Values | Notes |
|---|---|---|
| `independent` | `true`/`false` | `true` = tagged and tested independently |
| `used_by_otel` | `true` | Set if the module is used by the OTel agent build |
| `should_tag` | `false` | Set for internal tooling modules that shouldn't be tagged |
| `should_test_condition` | `never`/`always` | Override test behavior |
| `lint_targets` / `test_targets` | list of paths | Override default targets |

Most modules use `default` (no special config) or just set `used_by_otel: true`.

### Step 5: Report results

Report whether the task succeeded or failed. If it failed, read the error output and help fix the issue.

## Important Notes

- All changes must be committed before running `create-module` — the task may restore files on failure.
- After creation, the module is automatically added to `go.work` and all necessary `replace` directives are set up.
- To see the current module list: `modules.yml` or `dda inv modules.show`.

## Usage

- `/create-go-module` — Interactive: prompts for the module path
- `/create-go-module comp/core/mything/def` — Pre-fills the path
