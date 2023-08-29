
# [HOWTO] Fix the linter errors for your team

## Context

Right after `7.48` merge freeze [we enabled `revive` and `gosimple` linters](https://github.com/DataDog/datadog-agent/pull/18805). To avoid breaking `main` they are only linting commits made after [f40667d](https://github.com/DataDog/datadog-agent/commit/f40667d3841c6339be0d00d53e54a4a63f43f11e).

We still want to fix the `revive` and `gosimple` errors that came before [f40667d](https://github.com/DataDog/datadog-agent/commit/f40667d3841c6339be0d00d53e54a4a63f43f11e).


## Prerequisites

The version of `go` and `golangci-lint` can have a big impact on the output of the linters. You'll need to have a setup close to the CI's to have the same output. Please meet the following conditions before running the commands:
- Your `go` version (run `go version`) is identical to the CI's (content of the `.go-version` file).
- Your `golangci-lint` version is identical to the CI's (look for `golangci-lint` in the `internal/tools/go.mod` file).
- You did not install `go` using `brew` (`which go` path shouldn't contain `homebrew`).
- Install the requirements with `python3 -m pip tasks/fix_revive_linter/install requirements-need-fixing-linter.txt`

## Fixing the `gosimple` linter

Already done in [#18884](https://github.com/DataDog/datadog-agent/pull/18884).


## Fixing the `revive` linter

Run the command

```bash
inv -e need-fixing-linters --filter-team "@DataDog/your-team" --filter-linters "revive"
```

Note: The linter is running every combination OS x Arch we're linting in the CI so it's normal for it to take a bit of time on the first run (should be faster after because some of it is cached).

Manually fix every lines in the command output create a PR with your fixes.
