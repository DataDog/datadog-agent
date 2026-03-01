---
name: test-agent
description: Run Go tests for the Datadog Agent using dda inv test. Use when running unit tests, after code changes, or when the user requests tests. Supports test options like --targets, --coverage, --module. Requires dev container to be running.
argument-hint: [test options]
allowed-tools: Bash(.claude/skills/test-agent/scripts/test.sh *)
---

# Test Agent

Runs Go unit tests for the Datadog Agent in a dev container from the current working directory.

## Prerequisites

The dev container must be running. If not, use `/start-dev-container` first.

## Usage

The skill automatically:
1. Computes path relative to main repo root
2. Changes to the current working directory inside the container
3. Executes tests

This works seamlessly in both the main repo and worktrees.
If you are working in a worktree, it must have been created [in relative mode](https://git-scm.com/docs/git-worktree#Documentation/git-worktree.txt---relative-paths).

## Arguments

Pass any arguments supported by `dda inv test`:
- `--targets=./pkg/aggregator`: Test specific package(s)
- `--module=<module>`: Test specific module
- `--coverage`: Enable coverage reporting
- `--race`: Enable race detection
- `--flavor=base|iot|serverless`: Specify agent flavor

## Examples

```
/test-agent
/test-agent --targets=./pkg/aggregator
/test-agent --targets=./pkg/collector --coverage
```

## Implementation

Executes: `~/.claude/skills/test-agent/scripts/test.sh $ARGUMENTS`
