---
name: create-invoke-task
description: Create a new Python invoke task for the dda CLI
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[namespace] [task-name]"
---

Create a new invoke task accessible via `dda inv <namespace>.<task-name>`.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides values, skip those questions.

1. **Task name**: The function name (e.g. `build`, `refresh_assets`). Underscores become hyphens in the CLI.

2. **Namespace**: Which task module does it belong to?
   - **Existing module** — Add to an existing file in `tasks/` (e.g. `agent`, `linter`, `dogstatsd`). Check `tasks/__init__.py` for the full list.
   - **New module** — Create a new `tasks/<name>.py` file and register it.

3. **What does the task do?**: Brief description (becomes the docstring / help text).

4. **Parameters**: What CLI flags does the task need? (name, type, default value, description)

### Step 2: Read a reference

Read an existing task file matching the desired complexity:
- **Simple task** (no params): `tasks/auth.py`
- **Task with params**: Pick any task from `tasks/agent.py` or `tasks/linter.py`
- **Registration**: `tasks/__init__.py` — to see how modules and tasks are registered

### Step 3: Create the task

Add the `@task` decorated function to the appropriate file. Follow the patterns from Step 2.

Key rules:
- First parameter is always `ctx` (the invoke `Context`)
- Use `ctx.run("command")` to execute shell commands
- Docstring first line = short description shown in `dda inv --list`
- Import from `invoke`: `task`, `Exit` (for errors), `Context`

### Step 4: Register (new modules only)

If you created a new module file, edit `tasks/__init__.py`:

1. Add the import in the `from tasks import (...)` block (alphabetical order)
2. Register it:
   - **As a namespace** (most common): `ns.add_collection(<module>)` — creates `dda inv <module>.<task>`
   - **As a root task**: `ns.add_task(<task>)` — creates `dda inv <task>`

If adding to an existing module, no registration is needed — invoke auto-discovers `@task` functions.

### Step 5: Verify

Test the task:
```bash
dda inv <namespace>.<task-name> --help
```

## Usage

- `/create-invoke-task` — Interactive: prompts for all details
- `/create-invoke-task agent my-task` — Pre-fills namespace and name
