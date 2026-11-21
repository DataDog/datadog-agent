---
name: onboardhealthsignal
description: Use when an engineer wants to onboard a new Agent Health signal (health issue type), turning a customer-side failure, its root cause and a repro into agent detection, tests and a draft PR under comp/healthplatform.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, AskUserQuestion
argument-hint: "[free text | --inputs <file.md>] [--path A|B] [--create-pr] [--out-dir <dir>]"
---

# onboardhealthsignal

Turn the engineer's inputs into reviewable DRAFT PR(s) that onboard one Agent Health signal, with test evidence. Scope v1: Path A (module + check) and Path B (an owner component reports directly) in datadog-agent, plus the dd-source catalog slice. Path C (backend service) is out of scope: say so and stop.

**Violating the letter of these rules violates the spirit.** `<skill-dir>` is this skill's base directory.

## Safety gates (hard)
- Never push, mark ready, merge, run AWS e2e, or run `ddr conductor` without an explicit yes to that action in this run. Without `--create-pr`, write the PR body to `<out-dir>/pr-body-<pkg>.md` (default `<worktree>/../`) and stop (dry-run).
- Work only in a git worktree made by `preflight.sh`, never in the user's checkout.
- No person names, `@handles` (team handles like `@DataDog/x` are fine), secrets, or em dashes in any output, code comment, or PR text. No skill or script names in repo code, comments, or release notes (only the `TODO(onboardhealthsignal)` scaffold marker, and only while `detection` is `todo`).
- Do not hide that an agent is running: if a tool edits a tracked file because it detected an agent (turbo appends to `AGENTS.md`), revert it with `git checkout -- <file>`; do not unset identity variables.

## Arguments
The harness appends a trailer `ARGUMENTS: <text>` after this skill text. It is plain text; nothing parses it for you. Parse it first:
- `--inputs <path>`: read that file (headings `## I1`, `## I2`, `## I3`), ask nothing, and write every gap to the report as an open question.
- `--path A|B` forces the path; `--create-pr` allows the PR step after an explicit yes; `--out-dir <dir>`; `--ack-drop-category` acknowledges a prod-dropped category.
- Any other text is free-text input; no trailer means a fully interactive interview.
Interactive: echo a structured summary of I1 to I3 plus the parsed flags and get confirmation before generating. Inputs mode: nobody can confirm, so list every inference as an assumption in the report.

## Inputs
Interview one question at a time (AskUserQuestion for enumerable answers); accept pasted tickets or PR links and ask only for gaps.
1. **I1 Code location:** where the root cause is detected or detectable; does detection already exist? **False positives:** which nodes or hosts may report, and what cheap signal says "this host should have the thing" (a presence gate beyond a cluster-wide config flag). **Timing:** must the issue appear and clear in step with an existing event (a Path B hint), or is a periodic evaluation fine?
2. **I2 Customer failure:** why it fails, who is affected (OS, k8s, versions), impact, the customer-side fix. **Does recovery of the observable mean the impact is fixed?** If the consumer caches state at startup, a restart or action is needed only when the dependency was absent at start: say so conditionally, never promise auto-resolve.
3. **I3 Repro and proof:** repro steps, the observable the Agent can read, proof tier. **Does the endpoint merely accept a connection, or answer a request?**

| Input | Drives |
|---|---|
| I1 | Path A vs B, package, build tag, check kind, `id_scope`, `multi_instance`, presence gate |
| I2 | IssueName, category, severity, text, remediation, `recovery`, catalog text |
| I3 | context keys, probe fidelity, tests, proof tier |

## Detection and recovery modes
- Remediation step 1 is a literal diagnostic command; every step is an action. A search instruction is allowed only when the file path varies by install, and then it names the exact string and the default paths per OS. Log steps give `kubectl logs` for Kubernetes plus `journalctl` or the default path per OS. States with different causes get their own text and test case.
- `detection: todo` registers no check, writes no release note, skips the catalog slice, and recommends `changelog/no-changelog`.
- `detection: partial`: the check is registered and tested but a stated requirement (for example attribution to a file or service) is NOT met. The limitation goes into the release note, the issue description (never a remediation step), the PR "What does this PR do" and the report, not only Additional Notes.
- `detection: implemented`: every stated requirement is met.
- `recovery`: `automatic`, `restart`, `manual`, `conditional` (restart only if the dependency was absent at agent start), or `held_window` (a historical condition held for `hold_minutes`: the note and issue text never say it clears on its own). Prefer gating on the consumer's own startup-cached state so the issue resolves exactly when a restart fixes the impact.

## Phases (each ends with a gate; stop and report on failure, quoting output verbatim)
**P0 Preflight.** Run helper scripts with `bash` and read `$?` (no pipes). `bash <skill-dir>/scripts/preflight.sh toolchain`, `auth`, then `cache` (prints and sets a shared bazel disk cache so fresh worktrees build warm) and `bazel-shutdown-idle` (lists idle bazel servers of other worktrees and stops only those; never one in use). Worktree: `preflight.sh worktree <clone> <ref> <dest>` (`<ref>` is `origin/main` or a pinned SHA with `NO_FETCH=1`; `--sparse` for read-only work). Then `rules <worktree>` and READ from that worktree: `comp/healthplatform/AGENTS.md`, `.agents/skills/create-pr/SKILL.md`, `test/new-e2e/tests/agent-health/.claude/skills/write-agent-health-e2e/SKILL.md`. They win over this skill; note the drift.
**P1 Interview** as above.
**P2 Classify and dedupe.** One call for the neighborhood: `bash <skill-dir>/scripts/repo_facts.sh <worktree> <keyword-or-dir>` (neighbor modules, config schema hits, feature and presence helpers, severities of siblings in the same category). Flag overlap with existing types; reuse an existing detector or helper or say why not. Derive names (`references/known-facts.md`). A `configuration` or `autodiscovery` category is dropped in prod: get acknowledgement. Path B when an existing component already knows the condition and the issue must track its event; Path A when a periodic check can evaluate it (`references/pitfalls.md`, "Path B").
**P3 Generate.** Read `references/pitfalls.md` first. Decide `path`, `id_scope`, `multi_instance`, `needs_config`, `consecutive_failures`, `recovery`, `build_tag`, `detection`. Before choosing `todo`, try the smallest host-level detection from already-linked dependencies and judge it with the sufficiency rubric in pitfalls.md. Write the spec (`references/spec-fields.md`), run `python3 -I <skill-dir>/scripts/render_templates.py --spec spec.json --worktree <wt>`, keep its printed `go_test_tags`, `integration_test_name`, `pr_title_suffix`, `pr_first_paragraph` (`run_hint` is a hint only: `verify_all.sh` probes `dda inv test --help` and is the single entry point). **Immediately after rendering, start the catalog slice (P6) in the background** (it needs only the built issue); then implement `detectCondition` (or the owner edit for Path B, guided by `references/path_b.md`) while it runs.
**P4 Verify (one entry point).** `cd <wt> && dda inv schema.codegen` once, then `bash <skill-dir>/scripts/verify_all.sh --path A|B --pkg <pkg> --owner-dirs <dirs> --worktree <wt> --catalog-dir <dir> --out-dir <out>`. It runs gazelle (minutes; it waits on the exit line), then the package tests, modified-package tests, linter, mutation check, `deps_delta`, `baseline_check` and catalog gates in parallel, and prints one JSON summary with per-gate logs. Any failed gate blocks the PR. Then `python3 -I <skill-dir>/scripts/check_signal.py --worktree <wt> --pkg <pkg> --json`. Manual commands and flag probing (`dda inv test --help`) are in `references/pitfalls.md`.
**P5 Proof.** Tier 1 (default, free): unit tests, the registration test (Path A: literals only, no import of the package; asserts category, severity, issue name and text on the wire), module test, a resolve proof (scheduler or `scheduler/mock`, injectable interval and hold window), and for Path B the owner adapter tests plus the wiring test (a requirement, recipe in `references/proof-tiers.md`). Run the mutant table for your path (`proof-tiers.md`) and list every survivor in the PR body. Tiers 2 and 3: `references/proof-tiers.md`; confirm cost and access first.
**P6 Catalog slice** (`references/catalog.md`): skip while `detection: todo`. Otherwise dd-source template plus fixtures from the real `BuildIssue`, formatted by the real formatter, jest run per the recipe. If a gate cannot run, mark the slice BLOCKED; never hand-splice generated TypeScript; the PR body says eslint was not run if a shim was used.
**P7 PR.** Follow the worktree's `create-pr` skill. "What does this PR do" lists only behavior with code and tests, plus any `partial` limitation. Verification bullets come from `scripts/verification_bullets.py` over the FINAL logs. Every command or UI claim in remediation text must exist in the tree (grep) or be marked UNVERIFIED in the body. The release note states platform scope (`platforms`). The renderer prints `codeowners_recommendation` and `pr_review_checklist`: EMIT the exact recommended line (derived from the nearest sibling) in the report and list it, plus the README module-table row, under a "Review checklist" heading in the PR body. Never add the CODEOWNERS line silently, in any mode. Never claim all mutants were killed unless each failed a test (a compile error is not a kill). Write `final.diff` and `catalog.diff` to the out-dir. No backend service names in repo code (say "the intake"). Interactive: show the body and ask before `gh pr create --draft`. Inputs or dry-run mode: build the body from `git add -N` plus `git diff`, write it to the out-dir, stop.
**P8 Report.** Commands run with results, tier reached, every `warn`, probe fidelity, severity compared with siblings, and open questions: severity and category, co-owner, the backend keying claim (UNVERIFIED), Windows scope, and what the issue cannot say.

## Red flags and rationalizations
| Thought | Reality |
|---|---|
| "Unit tests pass, skip the registration test" | A missing `bundle.go` import ships the fallback proto and still passes unit tests (check 17). |
| "Register the check now, detection comes later" | A registered check that cannot report costs a goroutine and ticker everywhere. `todo` registers none. |
| "The catalog fills in title and severity" | The resolver is off in most prod orgs and never fills severity; thin reports are dropped. |
| "It auto-resolves, say so" | Only if the consumer re-detects. Read its init path first. |
| "One failed probe is enough" | Transient failures flap. Return an error, or use `consecutive_failures`. |
| "Hysteresis plus restart is fine" | The scheduler seeds persisted ids and resolves any id the first run does not return: return an error until confirmed. |
| "Read the counter by its string name" | A rename silently kills the check. Test against the real variable or a shared constant. |
| "Hand-edit BUILD.bazel, fixtures, or generated TS" | Gazelle, the generator, and the real formatter exist because hand edits drifted. |
| "Docs said X" | AGENTS.md at the worktree SHA and worker code win. |
| "Just push it, it's a draft" | Pushing and PR creation need explicit confirmation. |

Reference files: `references/pitfalls.md` (read before P3), `references/path_b.md` (owner-edit guide, read for Path B), `known-facts.md`, `spec-fields.md`, `catalog.md`, `proof-tiers.md`, `templates/`.
