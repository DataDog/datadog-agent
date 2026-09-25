# Repository command launchers

This directory contains repository-managed commands. DotSlash manifests begin with `#!/usr/bin/env dotslash`; other launchers may use different mechanisms. See the [development prerequisites and shell setup](../../docs/public/setup/required.md) for making these commands available.

## Validating DotSlash manifests

Run `dda inv linter.dotslash` to check every discovered manifest using the pinned DotSlash parser and repository policy. Add `--download` to fetch every platform artifact from each provider independently into fresh temporary caches and verify that the requested executable exists. Add `--smoke` to run configured checks through the native checkout launchers with cold and populated caches. Both options can be combined; failed validation prevents native execution.

Use `mise exec -- dda inv linter.dotslash` in a shell without mise activation. The validator requires the interpreter selected by `mise.toml`; it does not substitute a global version.

`--changed-since=<ref>` limits download and native checks to changes since the merge base with that Git ref, including staged, unstaged, and untracked files. Shared validation inputs and Windows shim changes select all manifests. Static checks always cover every discovered manifest, and an unavailable comparison ref fails the command. `--report=<path>` writes JSON results, including failures and skipped native checks.

CI runs static checks routinely and requires download verification followed by checks on Linux AMD64/ARM64, macOS AMD64/ARM64, and Windows AMD64 for relevant changes. Each provider is verified independently, so a functioning fallback cannot conceal an unavailable or incorrect primary provider. Download verification establishes consistency with the committed artifact metadata; upstream authenticity still requires independent review when updating an artifact.

The static CI job also tests the pinned interpreter's schema validation, cache reuse without provider requests, and fallback after a controlled provider failure. These integration tests use tiny standalone and archive fixtures; the download command does not repeat cache and fallback behavior tests for each real artifact.

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
