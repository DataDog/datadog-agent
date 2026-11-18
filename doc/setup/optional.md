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

[Pre-commit](https://pre-commit.com) provides earlier feedback for checks enforced by CI. The default profile checks protected branches, Go formatting and simplification, ShellCheck, GitHub Actions workflows, copyright headers, prohibited shell tracing, Python formatting and lint with Ruff. These checks validate staged contents without rewriting tracked files. No checks run on push or other events by default.

Install Go from `.go-version` and ShellCheck on `PATH`. DDA provides Ruff, and pre-commit installs the pinned actionlint environment. See [manual setup](manual.md#pre-commit-hooks) for details.

1. Install `pre-commit` by running the following command.

       ```
       dda self pip install pre-commit
       ```

1. Use `dda` to configure the pre-commit hooks.

       ```
       dda inv setup.pre-commit
       ```

Run setup again when migrating from the previous profile. It removes the previously installed pre-push hook before installing pre-commit. Commits still report each check separately. The adapters select their inputs from the index, including configuration deletions; they do not interpret `pre-commit run --all-files` as a request for repository-wide validation.

Use the existing explicit commands for broader validation or repairs, and review and stage any generated changes yourself:

| Validation | Explicit command |
| --- | --- |
| Go lint and tests | `dda inv linter.go`, `dda inv test` |
| Go module maintenance | `dda inv tidy`, `dda inv modules.check-all-replace`, `dda inv modules.validate` |
| Python, including mypy and Vulture | `dda inv linter.python` |
| Developer documentation links | `dda inv linter.docs-links` |
| GitLab configuration | `dda inv linter.full-gitlab-ci -t main --pre-push-linters --fail-fast` |
| Protocol buffers and CWS generation | `dda inv protobuf.generate`, `dda inv security-agent.cws-go-generate` |
| C/C++ formatting | `dda inv pre-commit.check-winclang-format` |
| Windows installer formatting | `dotnet format ./tools/windows/DatadogAgentInstaller` |
| Go maintenance and systemd generation | `dda inv linter.update-go`, `dda inv installer.generate-experiment-units --check` |
| E2E scenario imports and internet-access declarations | `go run ./tools/generate-scenario-imports/main.go -check`, `dda inv linter.e2e-internet-access` |
| Release notes | `dda inv linter.rst-releasenotes`, `dda inv linter.releasenote-unique-ids`, `dda inv linter.releasenote-reused-uids` |
| Release metadata newline | `dda inv pre-commit.check-release-json-newline` |

Buildifier and Rust formatting remain available through `bazel run //bazel/buildifier:test` and the repository's Bazel rustfmt aspect. CI remains authoritative for merge requirements. Startup optimization and complete-profile performance qualification are deferred.
