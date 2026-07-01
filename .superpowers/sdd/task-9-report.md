# Task 9 Report: CLI binary (`scenariorun`) + `dda lab` forwarder

## Files Created / Modified

### Created
- `test/e2e-framework/cmd/scenariorun/main.go` — cobra root with `list`, `describe [--json]`, `create <scenario>`, `action <scenario> <action>`, `destroy <scenario>`; per-scenario subcommands with flags generated at runtime via `registerFlagsSafe`.
- `test/e2e-framework/cmd/scenariorun/import_scenarios.go` — `registerScenarios()` calling `ec2host.Register()`.
- `test/e2e-framework/cmd/scenariorun/main_test.go` — two TDD tests (`TestDescribeCommandListsScenario`, `TestCreateCommandHasSchemaFlags`).
- `tasks/scenario.py` — `lab` task: builds `bin/scenariorun` and forwards args.

### Modified
- `tasks/__init__.py` — added `scenario` to the import block and `ns.add_collection(scenario)` to the collection block.

## TDD Evidence

### RED (before implementation)
```
# github.com/DataDog/datadog-agent/test/e2e-framework/cmd/scenariorun
cmd/scenariorun/main_test.go:18:10: undefined: rootCmd
cmd/scenariorun/main_test.go:33:10: undefined: rootCmd
FAIL  github.com/DataDog/datadog-agent/test/e2e-framework/cmd/scenariorun [build failed]
```

### GREEN (after implementation)
```
=== RUN   TestDescribeCommandListsScenario
--- PASS: TestDescribeCommandListsScenario (0.00s)
=== RUN   TestCreateCommandHasSchemaFlags
--- PASS: TestCreateCommandHasSchemaFlags (0.00s)
PASS
ok  github.com/DataDog/datadog-agent/test/e2e-framework/cmd/scenariorun  1.857s
```

## Describe Smoke Output

```
$ cd test/e2e-framework && go build -o bin/scenariorun ./cmd/scenariorun && ./bin/scenariorun describe --json | head -20
{
  "protocolVersion": 1,
  "scenarios": [
    {
      "name": "ec2-host",
      "description": "AWS EC2 VM with the Datadog Agent",
      ...
```

Both `"ec2-host"` and `"protocolVersion": 1` confirmed in output.

## Python Syntax Check

```
$ python3 -c "import ast, pathlib; ast.parse(pathlib.Path('tasks/scenario.py').read_text()); print('scenario.py: OK'); ast.parse(pathlib.Path('tasks/__init__.py').read_text()); print('__init__.py: OK')"
scenario.py: OK
__init__.py: OK
```

## API Adaptations

One adaptation required: **duplicate flag name collision**. `EC2HostParams` embeds both `AgentParams.Fakeintake bool` (scenario tag `use-fakeintake`) and `FakeintakeParams.Enabled bool` (also `use-fakeintake`). The shared `scenario.RegisterFlags` panics when pflag sees a duplicate name on the same `FlagSet`.

Resolution: introduced `registerFlagsSafe(s scenario.Schema, fs *pflag.FlagSet)` in `main.go` that deduplicates by name before calling `scenario.RegisterFlags`. The first occurrence wins (which is `AgentParams.Fakeintake` from index `[2,4]`). This is a CLI-local workaround; the schema itself accurately reflects both fields in its JSON output. A proper fix would be to reconcile the two `use-fakeintake` fields in `EC2HostParams` (likely by removing `Agent.Fakeintake` or redirecting it to `Fakeintake.Enabled`), but that is a pre-existing design issue in Task 7/ec2host params, not in scope for Task 9.

## Concerns

1. **dda wiring unverified**: `dda inv scenario.lab` was not verified at runtime (no `dda` invocation). The import and `ns.add_collection(scenario)` in `tasks/__init__.py` follow the `ai_sandbox` pattern exactly and both files pass Python AST parse. The namespace will be `scenario.lab` (i.e., `dda inv scenario.lab --args="list"`).

2. **Duplicate schema field**: `EC2HostParams` exposes `use-fakeintake` twice in the schema JSON. The CLI silently uses only the first occurrence for the flag; the second is dropped. This may confuse users reading the JSON schema vs. the CLI help. Should be fixed upstream in the ec2host params.

3. **No registry reset between tests**: the two tests both call `ec2host.Register()` without resetting the global registry between runs. Since the registry is a map and `Register` overwrites by name, duplicate calls are idempotent. This is safe but slightly fragile if a future test expects a clean registry state.

---

## Fix: Remove Duplicate `use-fakeintake` Field (Root Cause)

### Changes Made

1. **`test/e2e-framework/scenario/params/agent.go`**: Deleted `Fakeintake bool` field (was `scenario:"name=use-fakeintake,..."`) — dead field never used in `ToOptions()`.

2. **`test/e2e-framework/scenario/scenarios/ec2host/scenario.go:64`**: Changed `if p.Agent.Fakeintake || p.Fakeintake.Enabled {` to `if p.Fakeintake.Enabled {`.

3. **`test/e2e-framework/cmd/scenariorun/main.go`**: Replaced both `registerFlagsSafe(sc, sub.Flags())` and `registerFlagsSafe(asc, actSub.Flags())` with `scenario.RegisterFlags(...)`. Deleted `registerFlagsSafe` helper function and removed the `pflag` import.

4. **`test/e2e-framework/go.mod`**: `github.com/spf13/cobra v1.10.2` promoted from `// indirect` to direct dependency.

### Verification

```
go test ./scenario/... -v
ok  github.com/DataDog/datadog-agent/test/e2e-framework/scenario (cached)
ok  github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params 1.712s
ok  github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host 2.090s

go test ./cmd/scenariorun/ -v
ok  github.com/DataDog/datadog-agent/test/e2e-framework/cmd/scenariorun 1.849s

go build ./... && go vet ./scenario/... ./cmd/scenariorun/
(no output — clean)

./bin/scenariorun describe --json | grep -c use-fakeintake
1
```
