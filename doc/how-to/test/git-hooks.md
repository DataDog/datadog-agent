# Validate the default Git hooks

The default pre-commit checks must preserve staged and unstaged contents, apply no fixes, and finish within five seconds at p95 on representative Windows and Linux changes after tool setup. Pre-push and other events have no default validation.

Run the focused tests on both platforms:

```sh
python -m unittest discover -s tasks/unit_tests -p 'hooks*_tests.py' -v
```

Install pre-commit and the tools described in [optional development setup](../../setup/optional.md#pre-commit-hooks). Put the native tool binaries on `PATH`, including the pinned Go installation and a rustup installation with the pinned rustfmt component. A Mise shim that works in your trusted checkout can reject its configuration in a new temporary checkout. Use a full archive of the commit containing the proposed hooks and validators:

```sh
python tasks/hooks.py setup-buildifier
git archive --format=tar --output=.cache/hooks/qualification.tar HEAD
python tasks/hooks_benchmark.py .cache/hooks/qualification.tar --runs 20 --output .cache/hooks/latency.json
```

Run the benchmark on each platform's native filesystem. If `/tmp` is memory backed, set `TMPDIR` to an existing directory on your normal Linux workspace filesystem. The harness checks every tracked file's path and mode against the commit in the script's repository. It compares file contents and symlink targets with a fresh full archive from Git, which applies the commit's line-ending attributes when writing the archive. It renders from an isolated object view so local attribute overrides and replacement refs cannot change the recorded identity. If native Git needs a different checkout to read that commit, pass `--source-repository PATH`. Record the harness revision separately when it differs from the archived revision.

The temporary repository uses an isolated Git configuration and pre-commit cache. After installing Buildifier and the managed Python environment, an untimed preflight runs all nine hooks against sample files inside that repository. Each timed invocation uses `git hook run pre-commit`, including Git's hook launcher. Scenarios cover documentation, mixed languages with LF and mixed line endings, partially staged Python, and Go, Ruff, Buildifier, Rust, ShellCheck, actionlint, and copyright configuration changes. Ruff caching is disabled. Filesystem caches are not flushed. The fixture retains Git for Windows' default process-local metadata optimization and supports checkouts without symlink privileges.

The JSON report records preflight output and each invocation's exit code, output, classification, and preservation result. It checkpoints completed invocations before preparing the next scenario, so a later preparation error leaves a partial report identifying that phase. Timed scenarios include the first run, p95, and maximum. Every invocation compares the index tree, tracked unstaged changes, raw bytes of the scenario inputs, and the names of nonignored untracked files. The full-scope ShellCheck scenario can report existing lint findings when the other eight hooks pass. The harness requests ShellCheck's GCC output format so it can recognize diagnostics with a file location and an SC code. A skipped or missing hook, an unexpected failure, or a preservation failure stops qualification with a nonzero exit and a partial report.

Latency results must come from the exact separate-hook profile being qualified. Measurements from the earlier aggregate dispatcher do not qualify this configuration. Record the source revision, tool versions, operating systems, first-run and p95 timings, exit codes, and preservation results with each report.

When the checkout's parent is unwritable, ShellCheck's CRLF handling uses a temporary view inside the checkout. Repository-local sources and known ancestor configuration remain available. If an ancestor permits traversal but forbids listing, use absolute source paths for external dependencies in that directory.

The final functional integration run executes each scenario once to check execution and preservation on both platforms. It does not establish the five-second p95 target; performance qualification and optimization remain follow-up work.

## Recorded functional run

The separate-hook profile at `ab869500a5c7061ce8317d23fb6730f5bad21c42` was exercised on Windows 11 (NTFS) and Ubuntu 26.04 under WSL2 (ext4). Both runs used the full archive of `8a6a916ce3209780cb0a2fb6b56f0c6ed0a67919`, whose production hook configuration and validators are identical to that profile. The archive SHA256 was `d450d69fb51d0ba8cf83f0716ba4d61fb9dbe91b9fb98504d4993016dbd87c48`.

These are single-invocation functional observations, not p95 acceptance measurements. They used the earlier qualification harness (SHA256 `230c1549e0596a9e1c0623a17742c41ffb9d97021c4ea50644198022a0a8f82e`). Its saved outputs were checked to contain all nine hook results with no skips. All scenarios preserved the index and tracked working contents. The revised harness additionally checks archive completeness, isolates caller environment settings, runs an untimed preflight, retains each invocation, and checks untracked filenames. Its failure paths have separate Windows and Linux fixture tests.

| Scenario | Windows seconds | Linux seconds | Exit code |
| --- | ---: | ---: | ---: |
| `documentation` | 3.459 | 0.740 | 0 |
| `mixed` | 4.959 | 1.191 | 0 |
| `mixed-crlf` | 348.070 | 16.782 | 0 |
| `partially-staged` | 4.148 | 0.878 | 0 |
| `go-settings` | 7.954 | 3.617 | 0 |
| `python-settings` | 4.156 | 1.037 | 0 |
| `buildifier-settings` | 5.255 | 1.687 | 0 |
| `rust-settings` | 6.781 | 1.381 | 0 |
| `shellcheck-settings` | 9.774 | 6.656 | 1 |
| `actionlint-settings` | 3.734 | 1.065 | 0 |
| `copyright-rules` | 5.674 | 1.628 | 0 |

The ShellCheck settings scenario exposed existing repository findings, including missing dialect declarations and unquoted expansions; the other eight hooks passed. This result does not establish a lint-clean full repository. The mixed-line-ending ShellCheck path builds a normalized repository view, which accounts for the particularly slow Windows observation. The original five-second latency target remains unmet; timing optimization is deferred.

Both platforms used pre-commit 4.7.0, Go 1.26.11, ShellCheck 0.11.0, actionlint 1.7.12, Ruff 0.3.5, Buildifier 10.1.0, and rustfmt 1.8.0 from Rust 1.94.0 with edition 2024. The launcher used Python 3.12.7 on Windows and 3.12.16 on Linux. Tool installation happened before timing. Build and lint-result caches were not used; filesystem and antivirus caches were not flushed.

The revised harness (SHA256 `8276657e53bbda98ba633f6d6f1eb83f3ea0c2ddf0fe52b778e7a073e5c549d4`) subsequently completed the same eleven-scenario archive on Linux. Its untimed nine-hook preflight passed, every invocation preserved the index, tracked contents, and nonignored untracked filenames, and only the ShellCheck settings scenario was classified as existing lint findings. Seven harness regression tests passed on both platforms, including a CLI preparation failure that retained five earlier scenarios. This run validates the corrected orchestration; it does not replace the Windows observations above or establish p95 acceptance.
