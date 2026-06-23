---
name: update-otel-deps
description: Use when a user asks to update, bump, or troubleshoot OpenTelemetry Collector dependencies in datadog-agent, including OCB build failures, ddflareextension golden files, static quality gates, or OTel transitive dependency conflicts.
argument-hint: "[target-version]"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
---

Update OpenTelemetry collector dependencies in datadog-agent.

**Arguments:** `$ARGUMENTS` — optional `<target-version>` (e.g. `v0.125.0`). Note: `inv collector.update` always upgrades to the latest published OTel version; if a specific version is requested, a manual search-and-replace is needed instead (see Manual Step 1).

---

## Step 0 — Check the automated workflow first

A scheduled GitHub Actions workflow (`.github/workflows/collector-generate-and-update.yml`) runs every other Wednesday and automatically opens a draft PR with the OTel update. **Always check this before doing anything manually.**

### 0a — Find the most recent run

```bash
unset GITHUB_TOKEN
gh run list --workflow=collector-generate-and-update.yml --branch=main --limit=5 \
  --json databaseId,status,conclusion,createdAt,event,displayTitle
```

**If the latest run succeeded:** find and review the auto-generated draft PR (Step 0c), then use `/dd:ci:fix` to fix any remaining CI failures and undraft.

**If the latest run failed:** go to Step 0b to diagnose and fix before falling back to a manual update.

**If no run has ever succeeded or the workflow looks consistently broken:** check [#opentelemetry-agent](https://dd.enterprise.slack.com/archives/C086Z7E2A0Y) and then fall back to Manual Step 1.

### 0b — Diagnose a workflow failure

Check [#otel-agent-ops](https://dd.enterprise.slack.com/archives/C08R19TR3FS) for the failure notification posted by the workflow; it includes a direct link to the failed run. Then fetch the logs:

```bash
RUN_ID=<id from step 0a>
unset GITHUB_TOKEN
gh run view $RUN_ID --log-failed 2>&1 | grep -E "error|Error|failed|exit code|XDG|bazel|OCB" | head -40
```

**Common workflow failures and fixes:**

| Symptom in logs | Cause | Fix |
|---|---|---|
| `XDG_CACHE_HOME () must denote a directory in CI` or exit code 2 on `bazel mod tidy` | `Set up Bazel cache` step missing or not running before `collector.update` | Ensure the `.github/actions/bazel-cache` step appears before `Run Collector Update Script` in the workflow |
| Branch named `update-otel-collector-dependencies-` (empty version) | `os.environ` in the Python subprocess doesn't propagate back to the shell; the old `echo "OCB_VERSION=$OCB_VERSION"` reads a blank value | Ensure the workflow extracts the version from `tasks/collector.py` after the update: `OCB_VERSION=$(grep -m1 'OCB_VERSION = ' tasks/collector.py \| grep -oP '"\K[^"]+')`  |
| `permission denied` / HTTP 403 from `dd-octo-sts-action` | Workflow running from a non-`main` branch | The octo-sts policy only grants tokens on `main`; always trigger via schedule or `gh workflow run … --ref main` |

After fixing the workflow, commit to `main` and re-trigger:

```bash
unset GITHUB_TOKEN
gh workflow run collector-generate-and-update.yml --ref main
gh run watch
```

If the workflow infrastructure succeeds but the resulting changes break CI (golden file mismatches, OCB build errors, etc.), that means the OTel version bump itself needs manual fixes. Proceed to Manual Step 2 using the auto-generated PR's branch as your working branch — don't start a new branch from scratch.

### 0c — Find and review the auto-generated PR

```bash
unset GITHUB_TOKEN
gh pr list --search "Update OTel Collector dependencies" --state open \
  --json number,title,url,headRefName
```

Review the diff, run `/dd:ci:fix` to fix CI failures, then undraft and request review once CI is green.

> **If a specific target version was requested** that differs from what the workflow produced, fall back to Manual Step 1.

---

## Manual Step 1 — Run the update

Use this only when: the automated workflow has a persistent infrastructure failure that can't be fixed quickly, or a specific target version (not latest) is requested.

```bash
dda inv collector.update   # bumps go.mod + OCB YAML files to latest OTel version
dda inv collector.generate # regenerates OTel Agent code from the new manifests
dda inv components.lint-components --fix
dda inv modules.add-all-replace
dda inv tidy               # reconcile transitive dependencies
bazel run //:go_mod_tidy_all
dda inv generate-licenses  # update license inventory
```

If a **specific version** was requested, skip `inv collector.update` and do a repo-wide search-and-replace of the old version string (find it in `tasks/collector.py` — the `OCB_VERSION` / `OTEL_CONTRIB_VERSION` constants). Then run the remaining commands above.

After the commands finish, scan for any files the task missed:

```bash
git diff HEAD --stat
OLD=$(git diff HEAD -- tasks/collector.py | grep '^-OCB_VERSION' | grep -oP '"[\d.]+"' | tr -d '"')
rg "$OLD" -g "*.go" -g "*.yml" -g "*.yaml" -g "*.mod" -g "*.sum" -l
```

Manually update any remaining files that still reference the old version.

---

## Manual Step 2 — Fix ddflareextension runtime config test failures

This is the **most common failure**. The DD flare extension tests compare OTel runtime config output against golden files. Any upstream config field change breaks them.

Run the test locally to get the actual diff:

```bash
dda inv test --targets=./comp/otelcol/ddflareextension/impl/... --build-include=otlp -- -run TestGetConfDump
```

The failure output shows exactly which lines differ. Apply the diffs to the golden files in:
- `comp/otelcol/ddflareextension/impl/testdata/` (unit test golden files)
- `test/new-e2e/tests/otel/otel-agent/testdata/` (E2E test golden files — apply the **same changes**)

> The E2E testdata mirrors the unit testdata for runtime config fields. Keep them in sync.

---

## Manual Step 3 — Fix OCB build test failures

The CI job `datadog_otel_components_ocb_build` verifies OTel modules can be built with OCB (OTel Collector Builder). Run it locally:

```bash
test/otel/testdata/ocb_build_script.sh
# or find the CI definition:
rg "ocb_build" .gitlab/ -g "*.yml" -l
```

Check logs under `/tmp/otel-ci/` for the exact error.

**Common causes and fixes:**

| Cause | Fix |
|---|---|
| Breaking changes in OTel core collector API | Update the affected Agent OTel modules to adapt (e.g. `comp/otelcol/`) |
| Incompatible changes between Agent and upstream `datadogexporter`/`datadogconnector` | 1. Open a PR in `opentelemetry-collector-contrib` to point the upstream exporter at a commit version of Agent packages. 2. After merge, add `replace` statements in `ocb_build_script.sh` pointing to the contrib HEAD commit. |
| Missing OCB artifacts during collector release (transient) | Wait for upstream release artifacts to appear, or temporarily disable the CI job after discussing in [#opentelemetry-agent](https://dd.enterprise.slack.com/archives/C086Z7E2A0Y). |

---

## Manual Step 4 — Fix static quality gate failures

OTel upgrades often increase binary sizes (e.g. via gRPC version bumps in transitive deps). OTel upgrade PRs have a standing exemption from the binary size gates. To fix:

1. Find which gate(s) failed in CI output (they look like `static_quality_gate_*`).
2. Increase the breached `max_on_disk_size` / `max_on_wire_size` values in `test/static/static_quality_gates.yml`.
3. Commit the change and request review from [#agent-delivery-reviews](https://dd.enterprise.slack.com/archives/C06PQ6KD5BK).

---

## Manual Step 5 — Fix conflicting transitive dependencies

OTel collector updates dependencies more aggressively than datadog-agent. A version bump may force a newer version of a transitive dep (commonly `docker` or `k8s` packages) that has breaking API changes.

**Preferred fix:** Update Agent code to use the newer API required by OTel's transitive dep version. Look at the build errors after `inv tidy` to find which symbols changed.

**Fallback:** Pin the old version with a `replace` directive in the relevant `go.mod`:

```
replace github.com/docker/docker => github.com/docker/docker v24.0.x+incompatible
```

Then run `dda inv tidy` again. Use this only when upgrading the Agent code is not feasible in the same PR.

---

## Manual Step 6 — Validate and open a PR

```bash
dda inv linter.go --targets=./comp/otelcol/...                                        # lint the changed OTel components
dda inv test --targets=./comp/otelcol/ddflareextension/impl/... --build-include=otlp  # confirm tests pass
```

Open a draft PR to let CI catch any remaining failures. The PR title convention is:
`Update OTel Collector dependencies to v<VERSION>`

Once the PR is open, run `/dd:ci:fix` to automatically fetch, analyze, and fix any remaining CI failures. If issues are not covered by the troubleshooting steps above, post in [#opentelemetry-agent](https://dd.enterprise.slack.com/archives/C086Z7E2A0Y).

---

## Success checklist

- [ ] Automated workflow checked — succeeded (auto-PR reviewed) or failure diagnosed and fixed
- [ ] `dda inv collector.update` + `collector.generate` + `tidy` + `generate-licenses` all completed without error (automated or manual)
- [ ] No remaining references to the old OTel version in tracked files
- [ ] ddflareextension unit + E2E golden files updated and tests pass
- [ ] OCB build script succeeds (or known issue tracked)
- [ ] Static quality gate limits raised if breached (exemption approved)
- [ ] Draft PR open and CI green (use `/dd:ci:fix` for any remaining failures)
