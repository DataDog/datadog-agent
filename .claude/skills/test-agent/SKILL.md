---
name: test-agent
description: Run Go tests for the Datadog Agent using dda inv test. Use when running unit tests, after code changes, or when the user requests tests. Supports test options like --targets, --coverage, --module.
argument-hint: [test options]
allowed-tools: Bash(.claude/skills/test-agent/scripts/test.sh *)
---

# Test Agent

Runs Go unit tests for the Datadog Agent in a dev container with worktree isolation support.

## Usage

The skill automatically:
1. Detects if running in a worktree or main repo
2. Determines the appropriate container ID
3. Ensures the dev container is running
4. Executes tests in the container

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
