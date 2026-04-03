---
name: create-release-note
description: Create a reno release note for a PR or change
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[topic]"
---

Create a release note file using the reno format. Release notes are **mandatory** for all PRs unless labeled `changelog/no-changelog`.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the topic, skip that question.

1. **Topic**: A short kebab-case identifier for the change (e.g. `fix-ntp-timeout`, `add-gpu-metrics`). Used in the filename.

2. **Section**: Which section does this change belong to?
   - `features` — New functionality (e.g. "Add support for GPU metrics collection")
   - `enhancements` — Small improvements to existing features (e.g. "Improve log rotation performance")
   - `fixes` — Bug fixes (e.g. "Fix memory leak in forwarder")
   - `deprecations` — Deprecation notices (e.g. "Deprecate `log_enabled` in favor of `logs_enabled`")
   - `upgrade` — Breaking changes requiring user action (must include steps for users to identify if affected)
   - `security` — Security fixes or improvements
   - `issues` — Known issues or limitations
   - `other` — Miscellaneous (rarely used)

3. **Content**: A user-facing description of the change. This is read by customers, not developers. Ask the user to describe what changed and why.

4. **Target**: Which release notes directory?
   - `releasenotes/notes/` — Main agent (default, most common)
   - `releasenotes-dca/notes/` — Cluster Agent (DCA)
   - `releasenotes-installscript/notes/` — Install script

5. **APM-related?**: If the change affects `cmd/trace-agent` or `pkg/trace`, the content must be prefixed with `APM : ` and the topic should be prefixed with `apm-`.

### Step 2: Create the release note file

Generate the file using reno:

```bash
reno new <topic> --no-edit
```

Or for non-default directories:
```bash
reno --rel-notes-dir <directory> new <topic> --no-edit
```

This creates a file at `<directory>/notes/<topic>-<hash>.yaml` with a template.

Then **replace the template content** with only the relevant section. Remove all unused sections (don't leave them commented out — reno template comments are noisy).

**Final file format:**

```yaml
---
<section>:
  - |
    Description of the change written for end users.
    Can span multiple lines.
```

### Step 3: Content formatting rules

Release note content must follow these rules:

1. **ReStructuredText (RST) format**, NOT Markdown:
   - Links: `` `link text <https://example.com>`_ `` (NOT `[text](url)`)
   - Bold: `**text**`
   - Inline code: ``` ``code`` ``` (double backticks, NOT single)
   - Code blocks: use `.. code-block:: <lang>` directive

2. **User-facing language**: Write for Datadog customers, not developers. Describe the impact, not the implementation.

3. **Self-contained**: Each note must be readable independently. Don't reference other release notes.

4. **For APM changes**: Prefix with `APM : ` (e.g. `APM : Fix trace sampling rate calculation`).

5. **Multiple items**: Use separate list items in the same section:
   ```yaml
   fixes:
     - |
       Fix memory leak when collecting Docker container metrics.
     - |
       Fix panic in log agent when file is rotated during read.
   ```

### Step 4: Verify

Run the release note linter to validate:

```bash
dda inv linter.releasenote
```

This checks:
- Valid YAML structure
- Only known sections are used
- No empty items
- Valid RST formatting (no Markdown patterns)
- No Markdown links `[text](url)`, headers `#`, or single-backtick code

If linting fails, fix the issues and re-run.

## Section Guidelines

| Section | When to use | Example |
|---|---|---|
| `features` | Wholly new capabilities | "Add Windows support for the Process Agent" |
| `enhancements` | Improvements too small to be features | "Add PDH data to Windows flare" |
| `fixes` | Bug fixes | "Fix EC2 tags collection with multiple marketplaces" |
| `deprecations` | Deprecation notices | "The `log_enabled` config option is deprecated, use `logs_enabled`" |
| `upgrade` | Breaking changes needing user action | "The `--config` flag format has changed. Users must update..." |
| `security` | Security fixes | "Enforce bearer token authentication for API endpoints" |
| `issues` | Known issues | "Kubernetes 1.3 is not fully supported in this release" |
| `other` | Misc (rarely used) | "Update internal CI tooling" |

## When to skip (use `changelog/no-changelog` label)

- Documentation-only changes
- Test-only changes
- CI/CD configuration changes
- Code comment changes
- Developer tooling changes that don't affect the agent binary

## Usage

- `/create-release-note` — Interactive: prompts for all details
- `/create-release-note fix-ntp-timeout` — Pre-fills the topic
