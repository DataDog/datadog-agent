# Set up optional development features

-----

## Tab completion

<<<DDA_DOCS_TAB_COMPLETE>>>

/// tip
There is also limited tab completion support for the legacy `invoke` tasks. Here's an example:

```
echo "source <(dda inv --print-completion-script zsh)" >> ~/.zshrc
```
///

## Pre-commit hooks

The default [pre-commit](https://pre-commit.com) hook checks protected branches, Go formatting, ShellCheck, GitHub Actions workflows, Go copyright headers, prohibited shell tracing, and Ruff. Each check has its own hook ID, so it can be run or skipped independently. It checks staged contents without applying fixes. When an editor leaves CRLF that Git stores as LF, validators check the LF form without rewriting the working file. Pre-commit temporarily hides unstaged tracked changes and restores them after validation, including when a check fails. Pre-push and other events have no default checks.

Use Python 3.12 or newer, the Go version in `.go-version`, and `shellcheck` and `actionlint` on `PATH`. Setup installs a shared Python environment with PyYAML and the Ruff version pinned by [DDA's CI dependency group](https://github.com/DataDog/datadog-agent-dev/blob/main/pyproject.toml). Keep the Ruff pin in `.pre-commit-config.yaml` aligned with that group when updating CI tooling.

```sh
python tasks/hooks.py setup-actionlint
```

1. Install `pre-commit` by running the following command.

       ```
       dda self pip install pre-commit
       ```

1. Use `dda` to configure the pre-commit hooks.

       ```
       dda inv setup.pre-commit
       ```

The setup command also removes the old pre-push hook when upgrading an existing installation. No hook downloads tools during a commit. After changing a tool version, repeat the relevant tool setup before committing.

To run a hook explicitly, use `pre-commit run <id> --all-files` or `pre-commit run <id> --files <paths...>`. To skip a check for one commit, use `SKIP=<id> git commit`.

For direct validation outside pre-commit, use `python tasks/hooks.py <check> <paths...>`, or use `--all` to check all eligible tracked files. Changes to a check's configuration conservatively select all files in its scope. Deleted files are omitted, and deleted configuration files still trigger validation. Pre-commit does not pass deleted paths to hooks, so staged configuration deletions conservatively expand the scope even during manual pre-commit runs. Use the direct command with explicit paths to limit validation to those paths.

Comprehensive validation remains available through commands such as `dda inv linter.python`, `dda inv linter.go`, `dda inv test --targets=<package>`, `dda inv tidy`, `dda inv protobuf.generate`, and `dda inv security-agent.cws-go-generate`. Run generators explicitly, review their output, and stage the resulting changes before committing. CI continues to enforce repository requirements.
