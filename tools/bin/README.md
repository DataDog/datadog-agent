# Repository command launchers

This directory contains repository-managed commands. DotSlash manifests begin with `#!/usr/bin/env dotslash`; other launchers may use different mechanisms. See the [development prerequisites and shell setup](https://datadoghq.dev/datadog-agent/setup/required/) for making these commands available.

DotSlash manifests support Linux and macOS on AMD64 and ARM64, and Windows on AMD64. Each manifest defines its artifacts and optional native check; no separate tool registry is required.

## Running tools

Run commands directly from the activated shell, for example `buildifier -version`. Automation should invoke the checkout's launcher by absolute path, pass arguments as an array, and set the intended working directory.

Use `buildozer <arguments>` in place of the former `bazel run //:buildozer -- <arguments>` convenience target.

Add a `dotslash_tool` declaration when a Bazel action needs a manifest, then consume it through the [DotSlash adapter](../../bazel/dotslash/README.md). Run `bazel build //bazel/dotslash/tests:formatted` for a complete example. Direct commands need no Bazel declaration or file export.

## Validating DotSlash manifests

Run `dda inv linter.dotslash` to check every discovered manifest using the pinned DotSlash parser and repository policy. Add `--download` to fetch every platform artifact from each provider independently into fresh temporary caches and verify that the requested executable exists. Add `--smoke` to run configured checks through the native checkout launchers with cold and populated caches. Both options can be combined; failed validation prevents native execution.

Use `mise exec -- dda inv linter.dotslash` in a developer shell without mise activation. The validator requires the version pinned in `mise.toml` on `PATH`, regardless of how it was installed.

`--changed-since=<ref>` limits download and native checks to changes since the merge base with that Git ref, including staged, unstaged, and untracked files. Shared validation inputs and Windows shim changes select all manifests. Static checks always cover every discovered manifest, and an unavailable comparison ref fails the command. `--report=<path>` writes JSON results, including failures and skipped native checks.

Every manifest must cover the supported platforms and include its Windows launcher. Each provider is verified independently, so a functioning fallback cannot conceal an unavailable or incorrect primary provider. Download verification establishes consistency with the committed artifact metadata; upstream authenticity still requires independent review when updating an artifact.

For a platform with a GitHub release artifact URL, the first provider must be the matching repository GitHub release mirror URL. Additional fallback providers are allowed. This policy applies to every discovered manifest without assuming tool names, release-version syntax, asset filenames, or archive layouts. Other HTTPS sources do not require the GitHub mirror.

Run validator tests with `dda inv invoke-unit-tests.run --tests=dotslash,dotslash_task --directory=tasks/unit_tests`. Set `DOTSLASH_TEST_EXECUTABLE` to the pinned DotSlash executable to include schema, cache, and fallback checks.

## Optional native checks

Native checks are configured in the manifest's custom metadata:

```json
"metadata": {
  "native_check": {
    "command": ["buildifier", "-version"],
    "pattern": "(?m)^buildifier version: \\S+"
  }
}
```

The command must be an array of strings beginning with the manifest's `name`. The validator resolves that name to this checkout's launcher, using its `.exe` companion on Windows, and passes the remaining arguments literally without a shell. Choose a command that terminates without interactive input or external state changes. Each process has a five-minute timeout.

If `command` is specified, `pattern` must be a nonempty, valid Python regular expression. The command must exit successfully, and the pattern must match somewhere in its combined stdout and stderr. Use inline regex flags such as `(?m)` when needed. Avoid embedding version pins in the pattern unless the tool's check specifically requires them.

If the mapping or command is absent, the validator records native execution as skipped. Download validation still applies. Other metadata fields are reserved for future repository policies and provenance information; the current validator preserves them through the DotSlash parser.
