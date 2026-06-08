---
name: update-otel-deps
description: Update OTel collector dependencies in the Agent — runs inv collector.update/generate, fixes common test and build failures (ddflareextension testdata, OCB build, static quality gates, transitive dependency conflicts).
argument-hint: "[target-version]"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
---

Update OpenTelemetry collector dependencies in datadog-agent.

**Arguments:** `$ARGUMENTS` — optional `<target-version>` (e.g. `v0.125.0`). Note: `inv collector.update` always upgrades to the latest published OTel version; if a specific version is requested, a manual search-and-replace is needed instead (see Step 1).

---

## Step 1 — Run the update

```bash
dda inv collector.update   # bumps go.mod + OCB YAML files to latest OTel version
dda inv collector.generate # regenerates OTel Agent code from the new manifests
dda inv tidy               # reconcile transitive dependencies
dda inv generate-licenses  # update license inventory
```

If a **specific version** was requested instead of latest, skip `inv collector.update` and do a repo-wide search-and-replace of the old version string (find it in `tasks/collector.py` — the `OTEL_*_VERSION` constants). Then run `inv collector.generate`, `inv tidy`, `inv generate-licenses`.

After the commands finish, do a repo-wide grep for the **old version string** to catch any files the task missed:

```bash
git diff HEAD --stat             # review what changed
OLD=$(git diff HEAD -- tasks/collector.py | grep '^-OTEL' | grep -oP 'v[\d.]+' | head -1)
grep -r "$OLD" --include="*.go" --include="*.yml" --include="*.yaml" --include="*.mod" --include="*.sum" -l
```

Manually update any remaining files that still reference the old version.

---

## Step 2 — Fix ddflareextension runtime config test failures

This is the **most common failure**. The DD flare extension tests compare OTel runtime config output against golden files. Any upstream config field change breaks them.

Run the test locally to get the actual diff:

```bash
cd comp/otelcol/ddflareextension/impl
go test -tags otlp,test -run TestGetConfDump 2>&1
```

The failure output shows exactly which lines differ. Apply the diffs to the golden files in:
- `comp/otelcol/ddflareextension/impl/testdata/` (unit test golden files)
- `test/new-e2e/tests/otel/otel-agent/testdata/` (E2E test golden files — apply the **same changes**)

> The E2E testdata mirrors the unit testdata for runtime config fields. Keep them in sync.

---

## Step 3 — Fix OCB build test failures

The CI job `datadog_otel_components_ocb_build` verifies OTel modules can be built with OCB (OTel Collector Builder). Run it locally:

```bash
test/otel/testdata/ocb_build_script.sh   # if the script exists
# or check the gitlab CI definition:
grep -r "ocb_build" .gitlab/ --include="*.yml" -l
```

Check logs under `/tmp/otel-ci/` for the exact error.

**Common causes and fixes:**

| Cause | Fix |
|---|---|
| Breaking changes in OTel core collector API | Update the affected Agent OTel modules to adapt (e.g. `comp/otelcol/`) |
| Incompatible changes between Agent and upstream `datadogexporter`/`datadogconnector` | 1. Open a PR in `opentelemetry-collector-contrib` to point the upstream exporter at a commit version of Agent packages. 2. After merge, add `replace` statements in `ocb_build_script.sh` pointing to the contrib HEAD commit. |
| Missing OCB artifacts during collector release (transient) | Wait for upstream release artifacts to appear, or temporarily disable the CI job after discussing in [#opentelemetry-agent](https://dd.enterprise.slack.com/archives/C086Z7E2A0Y). |

---

## Step 4 — Fix static quality gate failures

OTel upgrades often increase binary sizes (e.g. via gRPC version bumps in transitive deps). OTel upgrade PRs have a standing exemption from the binary size gates. To fix:

1. Find which gate(s) failed in CI output (they look like `static_quality_gate_*`).
2. Increase the breached `max_on_disk_size` / `max_on_wire_size` values in `test/static/static_quality_gates.yml`.
3. Commit the change and request review from [#agent-delivery-reviews](https://dd.enterprise.slack.com/archives/C06PQ6KD5BK).

Example: find the failing gate name in CI, then:

```bash
# Edit the relevant entry in test/static/static_quality_gates.yml
# Raise max_on_disk_size / max_on_wire_size to just above the actual measured value
```

---

## Step 5 — Fix conflicting transitive dependencies

OTel collector updates dependencies more aggressively than datadog-agent. A version bump may force a newer version of a transitive dep (commonly `docker` or `k8s` packages) that has breaking API changes.

**Preferred fix:** Update Agent code to use the newer API required by OTel's transitive dep version. Look at the build errors after `inv tidy` to find which symbols changed.

**Fallback:** Pin the old version with a `replace` directive in the relevant `go.mod`:

```
replace github.com/docker/docker => github.com/docker/docker v24.0.x+incompatible
```

Then run `dda inv tidy` again. Use this only when upgrading the Agent code is not feasible in the same PR.

---

## Step 6 — Validate and open a PR

```bash
dda inv linter.go --targets=./comp/otelcol/...   # lint the changed OTel components
cd comp/otelcol/ddflareextension/impl && go test -tags otlp,test ./...   # confirm tests pass
```

Open a draft PR to let CI catch any remaining failures. The PR title convention is:
`Update OTel Collector dependencies to v<VERSION>`

If CI still has failures not covered above, post in [#opentelemetry-agent](https://dd.enterprise.slack.com/archives/C086Z7E2A0Y).
