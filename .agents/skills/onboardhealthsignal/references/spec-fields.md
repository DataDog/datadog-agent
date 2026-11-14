# Render spec (input to scripts/render_templates.py)

JSON object. Invented minimal example (all placeholder names):

```json
{"pkg": "widgetprobe", "path": "A", "issue_name": "Widget Probe Failed", "issue_id": "widget-probe-failed",
 "category": "availability", "location": "widget", "source": "widget", "severity": "MEDIUM",
 "check_kind": "periodic", "interval_minutes": 15, "check_source": "widget-probe",
 "context_keys": ["error", "target"], "title": "Widget probe cannot reach %q", "title_args": ["target"],
 "description": "The Agent cannot reach %q: %s.", "description_args": ["target", "error"],
 "summary": "Restore access to the widget endpoint.",
 "steps": ["Run `datadog-agent status` and check: {error}", "Verify `{target}` is reachable from the node."],
 "tags": ["widget"], "sample_context": {"error": "denied", "target": "/run/widget.sock"}}
```

| Key | Meaning |
|---|---|
| `pkg` | Go package name (lowercase, no hyphens); `pkg_dir` if the directory differs |
| `path` | `A` (module + check) or `B` (component reports directly; a spec key only, the renderer has no `--path` flag, while the skill's own `--path A|B` argument decides what you write here; Path B writes the issue file, its tests and the owner reporter, and no `module.go`, no `bundle.go` import) |
| `issue_name` | Title Case IssueName; IssueType is derived (lowercase, spaces to `_`, hyphens kept) |
| `issue_id` | kebab-case base for the instance ID |
| `category`, `location`, `source` | proto fields; check `category` against the prod drop set |
| `severity` | `LOW`, `MEDIUM`, or `HIGH` (never UNSPECIFIED) |
| `check_kind` | Path A only: `periodic`, `startup`, or `none` (reported by another component) |
| `interval_minutes` | periodic interval (0 = scheduler default); the generated module takes it as `newModule(deps, interval)`, so tests can inject a short one |
| `check_source` | unique scheduler registration key for periodic checks (not the report's `Source`; that is `source`) |
| `build_tag` | optional constraint on check.go; the noop file gets the negation |
| `context_keys` | snake_case keys BuildIssue reads (each gets a const, a default of `unknown`, and an Extra entry) |
| `title`, `title_args` | `fmt` format plus context keys |
| `description`, `description_args` | same |
| `summary`, `steps` | remediation; steps may contain `{key}` tokens. Step 1 is a literal diagnostic command in backticks (give `kubectl logs <agent-pod>` for Kubernetes plus `journalctl` or the default log path per OS when it reads logs). Every step is an action: a limitation never goes in a step. When states differ, write a separate spec per state-specific text or branch the text in `BuildIssue` and add an `issue_test.go` case per state |
| `step1_path_varies` | true only when the file path varies by install and step 1 is a search instruction: it then names the exact string to search (quoted) and at least two default paths per OS. The renderer prints a note otherwise |
| `co_owner` | feature team handle for the printed CODEOWNERS recommendation (`@DataDog/<team>`); the line is printed with `pr_review_checklist`, never added to the tree |
| `tags`, `extra_static` | tags list; fixed Extra entries (snake_case keys) |
| `sample_context` | values used in the generated tests |
| `release_note_condition` | phrase completing "reports X when ..." |
| `type_struct`, `module_struct` | override Go type names (defaults derive from `issue_name`: `Widget Probe Failed` gives `WidgetProbeFailedIssue` and `widgetProbeFailedModule`) |
| `needs_config` | true when the check reads agent config; the constructor then gets `deps.Config` and the noop and test helper follow |
| `id_scope` | `daemonset` (default) or `host`; see `pitfalls.md`. Path A only: it has no effect on Path B, where the owner adapter chooses the id (see `path_b.md`) |
| `owner_dir`, `owner_pkg` | Path B only: repo-relative owner directory and its Go package; the renderer writes the owner reporter `<owner_dir>/<issue_type>_health.go` and its tests there. The reporter takes an injectable store interface; `ObserveScan(ctx, ...)` checks `ctx.Err()` first; the Id is hashed per host with a hostname fallback (unit tested); it resolves ids persisted under another Id; `ResolveAll` serves the disabled-owner case; a nil store or nil reporter is a no-op. The wiring test recipe sits as a comment at the end of the generated test file and is NOT generated (write it; recipe in `proof-tiers.md`). Pass the same directory to gazelle, the tests and the linter (`--owner-dirs` in `verify_all.sh`) and to `check_signal.py` (`--owner-file`) |
| `multi_instance` | true for several occurrences per host; each gets its own issue ID via `instance.Key`. The key must be bounded (distinct keys equal backend issues) |
| `detection` | `todo` (default), `partial`, or `implemented`. `todo`: the module registers NO built-in check and NO release note is written (a registered check that cannot report only burns a goroutine and ticker; use `changelog/no-changelog`). `partial`: the declared `check_kind` is registered and the note is written, but a stated requirement is unmet; it REQUIRES `unmet_requirements`, and the limitation lands in the release note, the issue description (never a remediation step), and the printed `pr_title_suffix` and `pr_first_paragraph` (use them for the PR title and the "What does this PR do" paragraph, not only Additional Notes). `implemented`: every stated requirement is met; the scaffold keeps no TODO markers and no skipped scaffolds. Switch after implementing `detectCondition` and re-render only the wiring with `--only module,release-note` |
| `unmet_requirements` | list of short phrases naming each stated requirement the detection does not meet (required when `detection` is `partial`, for example "no attribution to a file or service") |
| `recovery` | `automatic` (default), `restart`, `manual`, `conditional`, or `held_window` (default when `hold_minutes` is set; needs `hold_minutes`; the note says the issue stays active for the window after the last sighting, and the renderer rejects an auto-clear recovery with `hold_minutes` or issue text that says it clears on its own) (restart only if the dependency was absent at agent start; a long-lived client that was reachable at start reconnects on its own). It sets the recovery sentence in the release note; never promise auto-resolve the consumer cannot deliver |
| `consecutive_failures` | hysteresis: report an instance only after N consecutive sightings (default 1). An instance seen but not yet confirmed makes the check return `errPendingConfirmation` (an error, not nothing), so the scheduler does not resolve a persisted real issue on the first run after a restart; there is no first-run flag, and a miss resets the count; see `pitfalls.md` |
| `hold_minutes` | keep reporting an instance for that long after it was last seen (a hold window for historical conditions); the clock is injectable so tests advance it and prove the resolve after expiry |
| `platforms` | list such as `["Linux", "macOS"]`; adds "Supported on ... only." to the release note (the release-note platform scope; the renderer notes its absence when detection exists) |
| `unix_socket_tests` | true when the detect tests use unix sockets (or `Geteuid`, chmod): adds `&& linux` to the detect test build line so a tag that is also in Windows builds still compiles there |
| `impact_variants` | `{context_key, when_nonzero, otherwise}`: conditional `impact` text (for example a different sentence at the exact boundary where nothing is lost) with a generated test per branch; `context_key` must be one of `context_keys` |
| `gate_config`, `gate_env` | config keys and environment variables that enable the check, used by the generated gate-through-config bundle test (active only with `detection: implemented`, no `build_tag`, and `consecutive_failures` 1; otherwise a skipped scaffold with a printed note) |

Key names that are Go predeclared words (for example `error`) become `<name>Value` variables automatically.

## Mapping from the interview
- I1 gives `path`, `pkg`, `check_kind`, `build_tag`.
- I2 gives `issue_name`, `category`, `severity`, `title`, `description`, `summary`, `steps`, `tags`.
- I3 gives `context_keys`, `sample_context`, and the detection body you write into `check.go` (`detectCondition`).

The renderer prints (JSON) `codeowners_recommendation`, `pr_review_checklist`, `go_test_tags`, `integration_test_name`, `run_hint`, `notes`, `pr_title_suffix` and `pr_first_paragraph`. `run_hint` is a HINT only: it assumes the `--test-args` flags that exist at current main. `scripts/verify_all.sh` probes `dda inv test --help` for the flags the worktree really supports and is the single verification entry point; use `integration_test_name` to confirm `--- PASS: <name>` in the log. The renderer emits string concatenation (not `fmt.Sprintf`) for a lone `%s`, which the repo linter requires.

Rendering writes `module.go`, `module_test.go`, `issue.go`, `check.go`, `check_test.go`, `detect_test.go` (a skipped scaffold while `detection` is `todo`; none when implemented), `check_noop.go` (only with a build tag), `issue_test.go` (exact-text assertions generated from `sample_context`), `comp/healthplatform/<pkg>_integration_test.go`, the `bundle.go` import (Path A), the Path B owner reporter and its tests (with `owner_dir` and `owner_pkg`), and a release note when `detection` is `partial` or `implemented` (no TODO markers remain once detection is implemented). It does NOT write the README module row or the CODEOWNERS line: both are in `pr_review_checklist`. It runs `gofmt -w` when available. It never writes `BUILD.bazel`.

`--only module,release-note` rewrites just those artifacts (use it after detection lands). Test and helper names derive from the type name (for example `TestWidgetProbeFailedSurvivesFullPipeline`), never from the lowercase package name.
