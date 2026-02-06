---
name: build-agent
description: Build the Datadog Agent using dda inv agent.build. Use when compiling the agent, after code changes, or when the user requests a build. Supports build options like --rebuild, --build-exclude, --flavor. Requires dev container to be running.
argument-hint: [build options]
allowed-tools: Bash(.claude/skills/build-agent/scripts/build.sh *)
---

# Build Agent

Builds the Datadog Agent in a dev container from the current working directory.

## Prerequisites

The dev container must be running. If not, use `/start-dev-container` first.

## Usage

The skill automatically:
1. Computes path relative to main repo root
2. Changes to the current working directory inside the container
3. Executes the build

This works seamlessly in both the main repo and worktrees.
If you are working in a worktree, it must have been created [in relative mode](https://git-scm.com/docs/git-worktree#Documentation/git-worktree.txt---relative-paths).

## Arguments

Pass any arguments supported by `dda inv agent.build`:
- `--rebuild`: Force a clean rebuild
- `--build-exclude=systemd`: Exclude systemd from build
- `--flavor=base|iot|serverless`: Specify agent flavor
- `--race`: Enable race detection
- `--development`: Development mode

## Examples

```
/build-agent
/build-agent --rebuild
/build-agent --build-exclude=systemd --flavor=base
```

## Implementation

Executes: `~/.claude/skills/build-agent/scripts/build.sh $ARGUMENTS`
