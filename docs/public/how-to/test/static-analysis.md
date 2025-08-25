# How to perform static analysis

-----

CI enforces static analysis checks for code, configuration, documentation, and more.

## Go

Go code can be analyzed with the `dda inv linter.go` command. This uses [golangci-lint](https://github.com/golangci/golangci-lint) which is an aggregator for several linters.

The configuration is defined in the [.golangci.yml](https://github.com/DataDog/datadog-agent/blob/main/.golangci.yml) file. The `linters` key defines the list of linters we enable.

/// tip
You can ignore linter issues on specific lines of code with the [nolint directive](https://golangci-lint.run/docs/linters/false-positives/#nolint-directive).
///

## Python

The `dda inv linter.python` command performs analysis on Python code. This uses [Ruff](https://github.com/astral-sh/ruff) for linting and formatting, and also ([for now](https://github.com/astral-sh/ruff/issues/872)) [Vulture](https://github.com/jendrikseipp/vulture) to find unused code.

/// tip
Ruff supports several ways to [suppress errors](https://docs.astral.sh/ruff/linter/#error-suppression) in your code.
///

## Other

All analysis tasks are prefixed by `linter.` and may be shown by running `dda inv --list`.
