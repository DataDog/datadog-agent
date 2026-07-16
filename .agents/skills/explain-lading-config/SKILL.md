---
name: explain-lading-config
description: Explains a lading.yaml config file from the regression test suite, using the lading Rust source as ground truth for field meanings and defaults.
user_invocable: true
argument-hint: "[experiment name]"
model: sonnet
---

# explain-lading-config

Explain what a lading regression test config does, grounded in lading source code.

## Quick Start

```bash
# 1. Verify the lading checkout exists and is on a known branch
bash .agents/skills/explain-lading-config/scripts/validate-lading-checkout.sh

# 2. Resolve $ARGUMENTS to a lading.yaml path (exact/substring/glob/path)
bash .agents/skills/explain-lading-config/scripts/resolve-lading-config.sh "$ARGUMENTS"

# 3. Read the resolved file, then ground every field in lading source
#    (see references/source-reading.md for the full strategy).

# 4. Write up the explanation following references/explanation-template.md.
```

Defaults must be resolved to concrete values, not function names. Full workflow below.

## Step 1: Validate lading checkout

Run `.agents/skills/explain-lading-config/scripts/validate-lading-checkout.sh`.

- Exit 0: script prints the current branch on stdout. If it is not `main`, warn
  the user that explanations are grounded in a non-main branch, then continue.
- Exit non-zero: the script prints a suggested `git clone` command on stderr.
  Relay that to the user and stop.

Override the checkout location with `LADING_DIR` if needed.

## Step 2: Determine target file

Use `.agents/skills/explain-lading-config/scripts/resolve-lading-config.sh` to
avoid ad-hoc matching. The script enumerates experiments under
`test/regression/cases/` (active) and `test/regression/x-disabled-cases/`
(disabled). Each experiment is a `<case>/lading/lading.yaml` addressed by its
case-directory name; disabled rows are flagged with a trailing `(disabled)`
column in the listing. `ebpf/cases/` (split-mode) and
`ebpf/config-only/cases/` are intentionally out of scope; if a user asks about
one, tell them this skill doesn't cover it yet.

The script handles path-like inputs, substring case names, and shell
globs (`*`, `?`).

**If `$ARGUMENTS` is provided:** run `resolve-lading-config.sh "$ARGUMENTS"`.
- Exit 0: stdout is the resolved absolute path; read it.
- Exit 3 (ambiguous): stderr lists candidates.
  - **≤ 4 candidates:** use `AskUserQuestion` to pick one, then read that
    path.
  - **> 4 candidates** (a broad substring like `i` can match 20+): do not
    try to force them into `AskUserQuestion`. Print the experiment names
    as a short bulleted list and ask the user to narrow the query and
    re-invoke `/explain-lading-config <name>`.
- Exit 2 (not found): stderr may include "did you mean?" suggestions — if
  present, offer the suggestions to the user via `AskUserQuestion` (up to
  4 options) or as a short list; if not, relay the error and stop.
- Exit 4 (wrong repo): the script is being run from outside the agent repo.
  Relay the error verbatim and stop — the user needs to `cd` into the repo.

**If the resolved path contains `/x-disabled-cases/`**, flag this explicitly
in the explanation — the experiment exists on disk but is not currently
executed by SMP. Otherwise a user may assume it's live.

**Reading very large configs:** multi-sender configs (e.g.
`uds_dogstatsd_20mb_12k_contexts_20_senders`, ~870 lines) are usually
block-copies of one template with a few fields varying (typically only
`seed`). Before a full `Read`, check size and duplication:

```bash
wc -l <path>                                    # scale check
grep -c '^  - ' <path>                          # top-level list entries
yq '.generator | length' <path> 2>/dev/null     # if yq is present
```

For highly-duplicated configs, `Read` only the first block (plus the
blackhole/target_metrics sections) and report the generator as
"N identical copies, seed differs" instead of walking every block. Spot-
check one later block to confirm uniformity.

**If `$ARGUMENTS` is omitted:** run `resolve-lading-config.sh` with no
argument. It emits `<experiment>\t<path>` lines for every discovered config.

Print the experiment names as a plain bulleted list to the user (preserving
the `(disabled)` markers) and ask them to type the name (or re-invoke the
skill with `/explain-lading-config <name>`).

## Step 3: Read the lading codebase for context

Before explaining, read the lading source files that ground the populated
sections of the config. The detailed strategy (variant-to-module mapping,
grep-before-Read invariants, fallback for renamed files) lives in
`references/source-reading.md` — read it now.

## Step 4: Explain the config

Write the explanation following the structure in
`references/explanation-template.md` (generator summary, aggregate load,
blackhole sinks, target metrics, source references). Read it now.
