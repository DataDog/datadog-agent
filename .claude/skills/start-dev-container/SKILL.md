---
name: start-dev-container
description: Start the dda dev container. Use when the dev container is not running and build or test commands fail.
argument-hint: ""
allowed-tools: Bash(dda env dev start)
---

# Start Dev Container

Starts the dda dev container for building and testing.

## When to Use

Use this skill when:
- `/build-agent` or `/test-agent` fails with connection errors
- You get "dev container not running" type errors
- Starting a new development session

## Examples

```
/start-dev-container
```

## Implementation

Executes: `dda env dev start`
