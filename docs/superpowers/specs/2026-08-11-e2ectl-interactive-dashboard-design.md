# e2ectl interactive dashboard & per-env loop

## Context

`e2ectl` (`test/e2e-framework/cmd/e2ectl/`) drives a local, no-Pulumi test
lifecycle — provision infra, install the agent, run the `go test` — from a
YAML test definition, with progress tracked in a JSON state file. Today's
interactive mode (`wizard.go`'s `promptForStage`) is a one-shot menu: it's
shown exactly once at startup, the chosen stage runs straight through
(`runLifecycle`), and the process exits. There's no way to see the current
state of an environment without reading its state file by hand, no way to
act on more than one stage per invocation, and no way to see or act on more
than one environment at a time.

This spec covers making the interactive mode a real navigable tool: a
dashboard across every environment e2ectl knows about, and a per-environment
loop that shows live status and lets you run any action repeatedly without
restarting the process.

## Goals

- See the state of every e2ectl-managed environment (provisioned? installed?
  which agent version? does the YAML's desired config match what's actually
  installed?) without reading JSON by hand.
- Navigate: pick an environment, act on it, see the result, act again — no
  process restart between actions.
- Generalize cleanly to any provisioner/installer combination, not just
  today's only pair (`kind` + `helm-k8s`) — adding a new one shouldn't
  require touching `cmd/e2ectl`'s own logic.

## Non-goals

- No new terminal UI dependency (bubbletea etc.) — stays plain
  `bufio.Scanner` over stdin/stdout, consistent with e2ectl's existing style.
- No cross-machine or shared state — still a single local checkout.
- No change to the non-interactive `--stage`/`--yes` CI path — scripted use
  is unaffected end to end.
- No reprovision-confirmation flow for infra. Selecting "provision" when
  infra already exists keeps today's behavior (a no-op with a message); only
  destroy gets a destructive-action confirmation gate.
- No per-provisioner status/describe abstraction (see "Generalizing
  provisioner status" below) — only the install side needs one today.

## Design

### 1. State file relocation

State files move from colocated-with-the-YAML
(`<config-basename>.state.json`) to one common, gitignored directory:
`test/e2e-framework/.e2ectl-state/<name>.state.json`, keyed by the env's
`name:` field. `name` is already required to be unique today (it's reused
for the kind cluster name, the fakeintake container name, and the Helm
release namespace/labels), so reusing it as the state filename introduces no
new uniqueness requirement.

Each state file gains one reserved top-level key, `"_source"`, holding the
absolute path to the YAML that produced it, so the dashboard can map a state
file back to its config without a separate registry. This key is
JSON-string-valued (not a `RawResources`-shaped entry). Verified safe: the
real consumer of these files, `SingleFileProvisioner[Env].ProvisionEnv` /
`assignImportKeys` (`testing/provisioners/file_provisioner.go`), walks the
target `Env` struct's *fields* looking for a matching entry key, and simply
ignores any entry that doesn't match a known field — an unrecognized
`"_source"` key is inert, not an error.

`defaultStatePath` changes from a pure function of the config path to one
that needs the parsed `TestDefinition.Name`. `main.go`'s `runRun` reorders
accordingly: load the YAML first, then compute the default state path if
`--state` wasn't given. `--state=<path>` continues to work as an explicit
override for anyone who wants a different location.

`.gitignore`'s existing `test/new-e2e/**/*.state.json` line is replaced with
a single `test/e2e-framework/.e2ectl-state/` entry. There is no existing
state file to migrate (the one real one, for `kind-nopulumi`, was destroyed
at the end of prior verification work).

### 2. Generalizing the "provisioned" check

`stagesCompleted` (`state.go`) currently detects "provisioned" by checking
for a `"kubernetesCluster"` key — that's `kind`'s specific resource key, not
a generic signal. It becomes: provisioned if any top-level key exists other
than `"agent"` and reserved metadata keys (currently just `"_source"`,
recognized by a `_` prefix). This works for any provisioner's resource key
without e2ectl needing a lookup table mapping provisioner type to resource
key name.

```go
func stagesCompleted(st envState) (provisioned, installed bool) {
	for k := range st {
		if k == "agent" || strings.HasPrefix(k, "_") {
			continue
		}
		provisioned = true
	}
	_, installed = st["agent"]
	return provisioned, installed
}
```

### 3. Generalizing install status/drift

Today's `agentConfigMatches` (`cmd/e2ectl/state.go`) unmarshals the state
file's `"agent"` entry straight into `compagent.KubernetesAgentOutput` — a
Kubernetes/Helm-specific shape. That's only valid for the `helm-k8s`
installer; a future installer (e.g. one that installs onto a plain host)
would write a differently-shaped `"agent"` entry, and this comparison would
misinterpret it rather than fail loudly.

The fix: move "is the installed config up to date, and how do I describe
it" into the `Installer` interface itself (`testing/installers/installers.go`),
next to `detect`/`install`, where the shape of its own output is already
known:

```go
// InstallStatus is an installer's read of its own "agent" state-file entry,
// relative to a desired InstallParams.
type InstallStatus struct {
	Summary  string // human-readable, e.g. "agent 7.81 / cluster-agent latest"
	UpToDate bool   // true iff the recorded config matches desired
}

type Installer interface {
	detect(envEntries map[string]json.RawMessage) bool
	install(ctx context.Context, envEntries map[string]json.RawMessage, p InstallParams) (json.RawMessage, error)
	status(agentRaw json.RawMessage, desired InstallParams) (InstallStatus, error)
}
```

Exported wrapper, mirroring `UpdateAgent`'s shape:

```go
// Status reports the installed agent's status relative to desired,
// resolving the installer the same way UpdateAgent does (explicit name, or
// auto-detect from envEntries). Returns a "not installed" status directly,
// with no installer resolution, if envEntries has no "agent" entry yet.
func Status(installerName string, envEntries map[string]json.RawMessage, desired InstallParams) (InstallStatus, error) {
	raw, ok := envEntries["agent"]
	if !ok {
		return InstallStatus{Summary: "not installed"}, nil
	}
	inst, err := resolve(installerName, envEntries)
	if err != nil {
		return InstallStatus{}, err
	}
	return inst.status(raw, desired)
}
```

`helmKubernetesInstaller.status` implements today's `agentConfigMatches`
logic (compare `LinuxNodeAgent`/`LinuxClusterAgent` version + namespace) and
renders a summary string; a future installer implements its own comparison
and summary against its own output shape. `cmd/e2ectl` calls
`installers.Status(...)` for both the up-to-date boolean (deciding whether
"install/update agent" is a no-op) and the display text (dashboard/loop
status lines) — it no longer imports or knows about
`compagent.KubernetesAgentOutput` at all. `state.go`'s `agentConfigMatches`
is deleted; its one call site in `wizard.go` is replaced with a call through
`installers.Status`.

### Generalizing provisioner status (considered, not doing)

The dashboard/loop's infra status line stays a plain "provisioned" / "not
provisioned" (from §2's generic key check) rather than gaining a
per-provisioner status/describe method the way installers just did. Only
one provisioner (`kind`) exists today, and unlike the agent (where the
*version* is the whole point), there's no version-like drift concept for
infra in this tool's scope — "does a cluster exist" is inherently binary.
Adding a parallel status abstraction for provisioners now would be
speculative generalization with no current consumer. If a future
provisioner needs richer status (e.g. "cluster exists but wrong Kubernetes
version"), this can be added the same way §3 was: as a method on
whatever the provisioner-side interface becomes, without touching the
dashboard/loop's contract with it.

### 4. CLI entry points

- `e2ectl` with **no arguments** → dashboard mode (new).
- `e2ectl run --config=<path> [--state=<path>] [--stage=...] [--yes]` →
  unchanged in every way, including the non-interactive `--stage`/`--yes`
  path used by scripting/CI. When run interactively (no `--stage`/`--yes`),
  it now enters the per-env loop (§6) directly for that one environment,
  rather than prompting once and exiting.
- `e2ectl destroy --config=<path> [--state=<path>]` → unchanged.
- The dashboard requires a terminal, using the same
  `term.IsTerminal(int(os.Stdin.Fd()))` check `runRun` already applies;
  invoking `e2ectl` with no args under non-interactive stdin is an error,
  matching today's `run` behavior in that situation.

### 5. Dashboard

On startup, the dashboard:

1. Globs `test/e2e-framework/.e2ectl-state/*.state.json`.
2. For each, reads `"_source"` and loads that YAML as a `TestDefinition`
   (skipping — with a printed warning, not a crash — any entry whose
   `_source` file is missing or fails to parse; state can outlive a
   YAML that was since moved/deleted).
3. Computes `stagesCompleted` + `installers.Status(...)` for each, and
   prints one line per environment:

   ```
   Known environments:
     1) kind-nopulumi   agent 7.81 / cluster-agent latest (up to date)   test/new-e2e/examples/kind_nopulumi_test.yaml
     2) other-env       not installed                                    test/new-e2e/foo/other_test.yaml
     o) open a config...
     q) quit
   ```

4. Picking a number loads that environment's `TestDefinition` + state path
   and enters the per-env loop (§6) with `cameFromDashboard = true` (see
   §6's `b) back to dashboard` item).
5. Picking `o)` prompts for a YAML path, loads it via the existing
   `loadTestDefinition`, computes its state path the normal way (§1 — this
   is the first time this env's state file would be written, so it won't
   exist yet), and enters the per-env loop the same way. Once any action
   writes to that path, it naturally shows up in the dashboard's glob on
   the next visit.
6. Picking `q)` exits.

### 6. Per-env loop

Replaces today's one-shot `promptForStage`. Entered either directly (`e2ectl
run --config=X` with no `--stage`/`--yes`) or via the dashboard. Each pass:

1. Re-read the state file from disk (so an external change, e.g. from a
   `go test` run using `installers.UpdateAgent` mid-suite, is picked up).
2. Print a status block:
   ```
   kind-nopulumi  (test/new-e2e/examples/kind_nopulumi_test.yaml)
     infra:  provisioned
     agent:  agent 7.65.0 installed, YAML wants 7.81 — drifted
     test:   ./examples/... -run TestKindNoPulumi
   ```
3. Print the fixed action menu:
   ```
   1) provision infra
   2) install/update agent
   3) run test
   4) destroy environment
   b) back to dashboard        (only shown if entered via the dashboard)
   q) quit
   ```
4. Read a choice:
   - `1`/`2`/`3` call the existing `doProvision`/`doInstall`/`doTest`
     directly (no longer gated behind a single `target` stage — each is
     independently invokable, any number of times). A stage that's already
     satisfied (e.g. `1` when infra exists, `2` when
     `installers.Status(...).UpToDate` is true) prints today's existing
     skip message and does nothing further. A failure prints the error
     (`fmt.Fprintln(os.Stderr, "error:", err)`) and returns to the same
     menu — **the loop never exits on an action failure**, so a transient
     error (network blip, port conflict) can be retried or worked around
     without restarting `e2ectl` and losing dashboard context.
   - `4` prompts "type the environment name to confirm destroy: ", compares
     the typed text to `def.Name`, and only calls the existing destroy path
     on an exact match; anything else cancels back to the menu with no
     effect. On success: if entered via the dashboard, return to the
     dashboard (the destroyed env's state file is gone, so it naturally
     drops off the next listing); if entered directly via `run --config`,
     stay in the loop (status now shows "not provisioned").
   - `b` (only offered when applicable) returns to the dashboard.
   - `q` exits the process.
5. After any of 1-4 completes (success or failure), loop back to step 1.

### Error handling summary

Every action (`doProvision`/`doInstall`/`doTest`/destroy) already returns an
`error` today; the only behavioral change is where that error is handled —
today it propagates out of `runLifecycle` and terminates the process
(`main.go` prints `"error:", err` and exits 1). In the new loop, the same
message is printed but control returns to the menu instead of exiting. The
non-interactive `--stage`/`--yes` path is unaffected — it still exits
non-zero on error, which is what CI needs.

## Files affected

- `test/e2e-framework/cmd/e2ectl/state.go` — generalize `stagesCompleted`
  (§2), delete `agentConfigMatches`, add glob-based dashboard discovery
  helper.
- `test/e2e-framework/cmd/e2ectl/config.go` — `defaultStatePath` becomes
  name-keyed against the new state directory; add a `stateDir()` helper
  (repo-root-relative, via the existing `repoRoot(ctx)` in `wizard.go`).
- `test/e2e-framework/cmd/e2ectl/wizard.go` — replace one-shot
  `promptForStage`/`runLifecycle` with the per-env loop (§6); add the
  dashboard (§5) as a new entry point, likely a new `dashboard.go`.
- `test/e2e-framework/cmd/e2ectl/main.go` — no-arg dispatch to the
  dashboard; reorder `runRun` to load the YAML before computing the default
  state path.
- `test/e2e-framework/testing/installers/installers.go` — add
  `InstallStatus`, the `status` method on `Installer` (and
  `helmKubernetesInstaller`), and the exported `Status` function (§3).
- `.gitignore` — replace `test/new-e2e/**/*.state.json` with
  `test/e2e-framework/.e2ectl-state/`.
- `test/e2e-framework/AGENTS.md` — document the dashboard, the per-env loop,
  the relocated state directory, and the `Installer.status` extension
  point for anyone adding a new installer.

## Testing / Verification

- Unit tests (`testing/installers` and `cmd/e2ectl`):
  - `helmKubernetesInstaller.status`: matching config → `UpToDate: true`;
    drifted version/namespace → `UpToDate: false`, summary mentions both
    recorded and desired values. (Supersedes today's `TestAgentConfigMatches`,
    moved and adapted to the new location/shape.)
  - `stagesCompleted`: unaffected cases (empty / provisioned / installed)
    still pass; add a case using a non-`kubernetesCluster` resource key to
    confirm the generalized check.
  - Dashboard discovery: a temp dir with a couple of state files (one with
    a valid `_source`, one with a missing/unparseable `_source`) → confirms
    the valid one is listed and the broken one is skipped with a warning,
    not a crash.
- `dda inv test --targets=./test/e2e-framework/...` and
  `dda inv linter.go --targets=./test/e2e-framework/...` clean.
- Manual verification against the real `kind-nopulumi` environment,
  mirroring the round-trip already done for config-drift re-install: fresh
  provision+install via the loop, confirm dashboard shows it correctly,
  edit `agentVersion` in the YAML, re-enter the loop and confirm drift is
  shown and re-install works, destroy via the loop's confirm-gated action
  and confirm it drops off the dashboard.
