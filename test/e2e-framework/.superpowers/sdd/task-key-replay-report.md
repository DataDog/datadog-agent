# Task: Capture import keys at create and replay them for Pulumi-free action hydration

## Mechanism

`environments.BuildEnvFromResources` resolves each importable env field's resource
by its import key — either a static `import:` struct tag or the key set by
`components.Export` → `Importable.SetKey` at Pulumi runtime. For `environments.Host`
there are no struct tags, so keys are only set during the Pulumi program run (i.e.
during `Provision`).

When `HydrateFromResources` was called later for `scenariorun action`, it created a
fresh env, skipping the Pulumi program, leaving keys empty → `BuildEnvFromResources`
errored with "has no import key set and no annotation".

The fix: snapshot keys immediately after `ProvisionWithResources` (when the provisioned
env's importable fields carry live keys from the Pulumi run), persist them in the local
state file, and replay them at hydration time via `SetKey` before calling
`BuildEnvFromResources`.

## Files Changed

| File | Change |
|------|--------|
| `testing/environments/environments.go` | Added `ImportKeys(env any) map[string]string` |
| `testing/standalone/standalone.go` | Changed `HydrateFromResources` signature to accept `keys map[string]string`; replays keys via `SetKey`, sets absent fields to nil; deleted `Hydrate` and `hydrateTimeout` |
| `testing/provisioners/pulumi_provisioner.go` | Deleted `StackOutputs` |
| `testing/utils/infra/stack_manager.go` | Deleted `GetStackOutputs` |
| `scenario/state.go` | Added `Keys map[string]string \`json:"keys"\`` to `ProvisionedStack` |
| `scenario/runnable.go` | `Create` captures `ImportKeys` and persists them; `RunAction` uses `ps.Keys` in `HydrateFromResources`; removed `Hydrate` fallback — missing local state is now a hard error |
| `testing/environments/import_keys_test.go` | New: unit tests for `ImportKeys` |
| `testing/standalone/hydrate_test.go` | New: round-trip test `TestHydrateFromResources_KeyReplayRoundTrip` |
| `scenario/state_test.go` | Updated `seedStack` + `TestSaveAndLoadRoundTrip` to include `Keys` field |
| `AGENTS.md` | Updated "Action hydration" section to describe key-replay mechanism |

## Round-trip Test

`TestHydrateFromResources_KeyReplayRoundTrip` in `testing/standalone/hydrate_test.go`:
- Creates `provisioners.RawResources{"dd-minImportable-alpha": <json>}`
- Passes `keys = {"Alpha": "dd-minImportable-alpha"}` (Beta absent)
- Calls `HydrateFromResources[minEnv]`
- Asserts `env.Alpha != nil` and `env.Alpha.Value == "alpha-value"`
- Asserts `env.Beta == nil` (absent from keys → nil field)

## Verify Output

```
go build ./...   → clean (no output)
go vet ./...     → clean (no output)
go test ./scenario/... ./cmd/scenariorun/ ./cmd/scenario-service/ ./testing/... → 10 ok packages, 0 FAIL
go build -o bin/scenariorun ./cmd/scenariorun && ./bin/scenariorun describe --json | head -3
→ { "protocolVersion": 1, "scenarios": [
```

## Grep-clean Confirmation

```
grep -rn "standalone\.Hydrate\b\|StackOutputs\|GetStackOutputs" --include="*.go" .
→ (no output)
```
