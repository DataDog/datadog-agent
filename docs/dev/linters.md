# Linting the datadog-agent repository

Welcome to the Datadog Agent linters documentation! Linters are tools that check our source code for errors, bugs, and stylistic inconsistencies. They're essential for maintaining code quality and consistency.

<!-- Those linters are running in the CI as required checks. -->

Here are the linters used on the datadog-agent repository depending on the language:

## Go

For Go, we're using [golangci-lint](https://golangci-lint.run/), a Go linters aggregator. Its configuration is defined [in the .golangci.yml file](https://github.com/DataDog/datadog-agent/blob/main/.golangci.yml).

The `linters` key defines the list of linters we're using:
https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/.golangci.yml#L65-L79

To run the linters locally, run `inv linter.go`.

> [!TIP]
> In your code, you can ignore linter issues on a line by prepending it with [the nolint directive](https://golangci-lint.run/usage/false-positives/#nolint-directive), for example,  `//nolint:linter_name`.
> Example [here](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/cmd/agent/common/import.go/#L252) and [here](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/cmd/agent/common/misconfig/global.go/#L27-L32).


## Python

For Python, we're using ([see invoke task](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/tasks/linter_tasks.py/#L17-L33)):
- [flake8](https://flake8.pycqa.org/en/latest), a style linter.
- [black](https://black.readthedocs.io/en/stable/), a code formatter.
- [isort](https://pycqa.github.io/isort/), to sort the imports.
- [vulture](https://github.com/jendrikseipp/vulture), to find unused code.

Their configuration is defined in both the [setup.cfg](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/setup.cfg) and the [pyproject.toml](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/pyproject.toml) files.

To run the linters locally, run `inv linter.python`.

> [!TIP]
> In your code, you can ignore linter issues on a line by prepending it with `# noqa: error_code`.
> Example [here](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/tasks/new_e2e_tests.py/#L40-L42) and [here](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/tasks/release.py/#L257).

## Troubleshooting

### Go

Q: I get a lot of errors locally that I don't get in the CI, why ?

A: This could have several causes:
- Your tool versions are not aligned with ours:
    - `go version` should output the same as [the repository .go-version](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/.go-version).
    - `golangci-lint --version` should output the same as [the repository internal/tools/go.mod](https://github.com/DataDog/datadog-agent/blob/dffd3262934a5540b9bf8e4bd3a743732637ef37/internal/tools/go.mod/#L8).
- You're testing OS specific code; running locally, the linters only get the results for your local OS.
- You didn't run `inv tidy-all` in the repository, making some dependencies in the remote env outdated compared to your local env.

### About the new-from-rev golangci-lint parameter

Introducing the `revive` linter in the codebase caused hundreds of errors to appear in the CI. As such, [the `new-from-rev` parameter](https://github.com/DataDog/datadog-agent/blob/fcb19ce078e7969d285565beec5d374c5fd623e1/.golangci.yml#L65-L68) was added to only display linter issues from changes made after the commit that enabled `revive`. [See the Golang documentation](https://golangci-lint.run/usage/faq/#how-to-integrate-golangci-lint-into-large-project-with-thousands-of-issues) for more information.

In a scenario where you have a legacy file hello.go with 100 linter issues, the new-from-rev parameter removes them all. But if you rename the file to hello_world.go, or move it to another folder, all the linter issues reappear. See [issue 4349](https://github.com/golangci/golangci-lint/issues/4349) in the golangci repo for more information.

This case added technical debt so [we removed it](https://github.com/DataDog/datadog-agent/pull/21266) and used the [the nolint directive](https://golangci-lint.run/usage/false-positives/#nolint-directive) instead.
