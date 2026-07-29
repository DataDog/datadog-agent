# Scenario Lifecycle Unification Implementation Plan (Phases 1–3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make one scenario definition authoritative for both the CLI and E2E tests — unify params/defaults, promote the lifecycle to a first-class testable API with pluggable env resolution, and prove the CLI path with cloud-free tests; then apply it to the real agent-health docker-permission suite.

**Architecture:** Build on the existing `scenario` package (schema/decode/flags/registry/state/runnable). Add `ApplyDefaults` + generic `NewParams[T]` so defaults come from tags in one place. Extract action execution into `DispatchAction` + an `EnvResolver[Env]` (state resolver for CLI, fixed-env resolver for tests). Add registry-keyed `Create/Action/Destroy` and a `scenariotest` bridge so a Go test reuses a scenario with one line. Actions are curated CLI affordances, not test-step mirrors.

**Tech Stack:** Go, module `github.com/DataDog/datadog-agent/test/e2e-framework`; Pulumi automation via existing `standalone`; `test/new-e2e` consumes the framework via a local `replace`.

## Global Constraints

- All framework code is in the **`test/e2e-framework`** module; run `go` commands **inside** it. This module is tag-light — use plain `go build`/`go test` (NOT `dda inv`). `test/new-e2e` code is verified with `go vet ./tests/...` from inside `test/new-e2e` (local `replace` resolves the framework).
- New `.go` files start with the 4-line Apache header (`Copyright 2025-present Datadog, Inc.`).
- Commit signing is broken: every commit uses `git commit --no-gpg-sign`.
- Branch: `unified-scenario-model-design` (already checked out).
- **Actions are curated CLI affordances, not test-step mirrors.** Do not convert test mutations into actions. Test logic stays as Go against `s.Env()`.
- Do not use `dda inv` for scenariorun; the binary is built/run directly.

## Existing pieces to build on (do not recreate)

- `scenario` package: `BuildSchema`, `Decode`, `RegisterFlags`/`CollectFlags`, `Register[Env]`/`Lookup`/`List`, `Describe`/`ProvisionVersion`, `state.go` (`ProvisionedStack`, `Save/Load/List/DeleteProvisionedStack`, `ErrNoProvisionedStack`, `toRawMessage`/`fromRawMessage`), `runnable.go` (`Runnable`, `genericRunnable[Env]` with `Create`/`RunAction`/`Destroy`).
- `Scenario[Env]{ Name, Description string; NewParams func() any; Provisioner func(any) (provisioners.TypedProvisioner[Env], error); Actions map[string]Action[Env] }`; `Action[Env]{ Description string; NewParams func() any; Run func(context.Context, *Env, any) error }`.
- `standalone.ProvisionWithResources[Env](ctx common.Context, stack string, p provisioners.Provisioner) (*Env, provisioners.RawResources, error)`, `standalone.HydrateFromResources[Env](ctx common.Context, resources provisioners.RawResources, keys map[string]string) (*Env, error)`, `standalone.Destroy`, `standalone.NewContext(dir string) *standalone.Context`.
- `environments.ImportKeys(env any) map[string]string`.
- `scenario/scenarios/ec2host` reference scenario; `scenario/params` (`AgentParams`, `FakeintakeParams`).

---

## Task 1: `ApplyDefaults` + generic `NewParams[T]`

**Files:**
- Modify: `test/e2e-framework/scenario/decode.go` (add `ApplyDefaults`)
- Create: `test/e2e-framework/scenario/params.go` (add `NewParams[T]`)
- Test: `test/e2e-framework/scenario/params_test.go`

**Interfaces:**
- Consumes: `Schema`, `Field`, `setValue`, `BuildSchema` (existing).
- Produces:
  - `func ApplyDefaults(s Schema, target any) error` — sets each field with a non-empty `Default` via `setValue`/`Index`; leaves fields without a default at zero.
  - `func NewParams[T any]() *T` — `p := new(T); s, err := BuildSchema(p); ApplyDefaults(s, p); return p` (panics on schema error — misuse at authoring time).

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/params_test.go
package scenario

import "testing"

type defaultsSample struct {
	OS      string `scenario:"name=os,default=ubuntu-22.04"`
	Count   int    `scenario:"name=count,default=3"`
	Enabled bool   `scenario:"name=enabled,default=true"`
	NoDef   string `scenario:"name=nodef"`
}

func TestApplyDefaults(t *testing.T) {
	var d defaultsSample
	s, _ := BuildSchema(&d)
	if err := ApplyDefaults(s, &d); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if d.OS != "ubuntu-22.04" || d.Count != 3 || !d.Enabled || d.NoDef != "" {
		t.Fatalf("defaults wrong: %+v", d)
	}
}

func TestNewParamsIsDefaulted(t *testing.T) {
	p := NewParams[defaultsSample]()
	if p.OS != "ubuntu-22.04" || p.Count != 3 || !p.Enabled {
		t.Fatalf("NewParams not defaulted: %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run 'TestApplyDefaults|TestNewParamsIsDefaulted' -v`
Expected: FAIL — `undefined: ApplyDefaults` / `NewParams`.

- [ ] **Step 3: Write minimal implementation**

Add to `scenario/decode.go`:
```go
// ApplyDefaults sets every field that declares a `default` tag to that default.
// Fields without a default are left at their zero value.
func ApplyDefaults(s Schema, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("ApplyDefaults: want pointer to struct, got %T", target)
	}
	elem := rv.Elem()
	for _, f := range s.Fields {
		if f.Default == "" {
			continue
		}
		if err := setValue(elem.FieldByIndex(f.Index), f.Kind, f.Default); err != nil {
			return fmt.Errorf("option %q: %w", f.Name, err)
		}
	}
	return nil
}
```

Create `scenario/params.go`:
```go
// <license header>

package scenario

import "fmt"

// NewParams returns a pointer to a T with all schema defaults applied. It is the
// blessed constructor for scenario params in Go code: it yields exactly the
// values the CLI/service produce for the same (empty) input, so Go tests and the
// CLI never drift. Panics if T is not a valid params struct (an authoring error).
func NewParams[T any]() *T {
	p := new(T)
	s, err := BuildSchema(p)
	if err != nil {
		panic(fmt.Sprintf("scenario.NewParams[%T]: %v", *p, err))
	}
	if err := ApplyDefaults(s, p); err != nil {
		panic(fmt.Sprintf("scenario.NewParams[%T]: %v", *p, err))
	}
	return p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run 'TestApplyDefaults|TestNewParamsIsDefaulted' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/decode.go test/e2e-framework/scenario/params.go test/e2e-framework/scenario/params_test.go
git commit --no-gpg-sign -m "feat(scenario): ApplyDefaults + generic NewParams[T]"
```

---

## Task 2: `Decode` = ApplyDefaults + overlay

**Files:**
- Modify: `test/e2e-framework/scenario/decode.go`
- Test: `test/e2e-framework/scenario/decode_test.go` (add cases)

**Interfaces:**
- Consumes: `ApplyDefaults` (Task 1).
- Produces: `Decode` unchanged signature (`func Decode(s Schema, values map[string]string, target any) error`) but now applies defaults via `ApplyDefaults` first, then overlays only the provided keys (validating enum/kind on provided; required check when a required key is absent). Behavior for callers is unchanged (defaults still applied, provided values still win), but defaulting lives in one place.

- [ ] **Step 1: Write the failing test**

```go
// append to test/e2e-framework/scenario/decode_test.go

func TestDecodeOverlaysDefaults(t *testing.T) {
	var got defaultsSample
	s, _ := BuildSchema(&got)
	// Only "os" provided; count/enabled must come from defaults.
	if err := Decode(s, map[string]string{"os": "debian-12"}, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "debian-12" || got.Count != 3 || !got.Enabled {
		t.Fatalf("overlay wrong: %+v", got)
	}
}

func TestDecodeOverlayOnAlreadyDefaulted(t *testing.T) {
	got := NewParams[defaultsSample]() // already defaulted
	s, _ := BuildSchema(got)
	if err := Decode(s, map[string]string{"enabled": "false"}, got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "ubuntu-22.04" || got.Count != 3 || got.Enabled {
		t.Fatalf("expected only enabled overridden: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestDecodeOverlay -v`
Expected: these may already PASS against the current `Decode` (it applies per-field defaults). Proceed to refactor so defaulting is centralized; the tests must still PASS after.

- [ ] **Step 3: Refactor `Decode` to reuse `ApplyDefaults`**

Replace the body of `Decode` in `scenario/decode.go` with:
```go
func Decode(s Schema, values map[string]string, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("Decode: want pointer to struct, got %T", target)
	}
	known := map[string]struct{}{}
	for _, f := range s.Fields {
		known[f.Name] = struct{}{}
	}
	for k := range values {
		if _, ok := known[k]; !ok {
			return fmt.Errorf("unknown option %q", k)
		}
	}
	// Defaults first, in one place.
	if err := ApplyDefaults(s, target); err != nil {
		return err
	}
	// Overlay only provided keys.
	elem := rv.Elem()
	for _, f := range s.Fields {
		raw, present := values[f.Name]
		if !present {
			if f.Required {
				return fmt.Errorf("missing required option %q", f.Name)
			}
			continue
		}
		if len(f.Enum) > 0 && !contains(f.Enum, raw) {
			return fmt.Errorf("option %q: %q not in [%v]", f.Name, raw, f.Enum)
		}
		if err := setValue(elem.FieldByIndex(f.Index), f.Kind, raw); err != nil {
			return fmt.Errorf("option %q: %w", f.Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the full scenario package**

Run: `cd test/e2e-framework && go test ./scenario/ -v`
Expected: PASS (new overlay tests + all existing decode/schema/flags/state tests).

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/decode.go test/e2e-framework/scenario/decode_test.go
git commit --no-gpg-sign -m "refactor(scenario): Decode applies defaults via ApplyDefaults then overlays"
```

---

## Task 3: ec2host uses defaulted `NewParams()`; drop `NewEC2HostParams`

**Files:**
- Modify: `test/e2e-framework/scenario/scenarios/ec2host/params.go`
- Modify: `test/e2e-framework/scenario/scenarios/ec2host/scenario_test.go` (usages of `NewEC2HostParams`)
- Modify: `test/e2e-framework/scenario/scenarios/ec2host/scenario.go` (the `NewParams func() any`, if it built a bare struct)
- Modify: `test/new-e2e/tests/scenario-model/ec2host_test.go` (usage)
- Modify: `test/e2e-framework/AGENTS.md` (remove the zero-value warning)

**Interfaces:**
- Consumes: `scenario.NewParams[T]` (Task 1).
- Produces: `func NewParams() *EC2HostParams { return scenario.NewParams[EC2HostParams]() }`. `NewEC2HostParams(os, arch)` is removed. The scenario's `NewParams func() any` returns `NewParams()`.

- [ ] **Step 1: Read the current file** to see the exact `NewEC2HostParams` and doc comment.

Run: `sed -n '1,80p' test/e2e-framework/scenario/scenarios/ec2host/params.go`

- [ ] **Step 2: Replace the constructor**

In `params.go`, remove `NewEC2HostParams(os, arch string) *EC2HostParams` and its zero-value doc comment; add:
```go
// NewParams returns fully-defaulted EC2HostParams (os=ubuntu-22.04, arch=x86_64,
// install-agent=true, …). This is the blessed constructor: it yields the same
// values the CLI produces for empty input. Override fields as needed.
func NewParams() *EC2HostParams { return scenario.NewParams[EC2HostParams]() }
```
Ensure `scenario.Scenario[environments.Host].NewParams` in `scenario.go` is `func() any { return NewParams() }`.

- [ ] **Step 3: Update call sites**

- In `scenario_test.go`, replace `NewEC2HostParams("ubuntu-22.04","x86_64")` / `NewEC2HostParams("ubuntu-22.04","arm64")` with `NewParams()` (override `.Arch = "arm64"` for the arm64 case).
- In `test/new-e2e/tests/scenario-model/ec2host_test.go`, replace `ec2host.NewEC2HostParams("ubuntu-22.04","x86_64")` with `ec2host.NewParams()`.
- In `test/e2e-framework/AGENTS.md`, delete the "**Go zero-value note:** … a bare `EC2HostParams{}` literal gets `Install=false` …" paragraph; replace with one line: "Use `ec2host.NewParams()` (fully defaulted) in Go; the CLI applies the same defaults via `Decode`."

- [ ] **Step 4: Build + test both modules**

Run:
```bash
cd test/e2e-framework && go build ./... && go test ./scenario/... -v
cd ../new-e2e && go vet ./tests/scenario-model/...
```
Expected: no `NewEC2HostParams` references remain (`grep -rn NewEC2HostParams test/` → empty); tests pass; vet clean.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/scenarios/ec2host/ test/new-e2e/tests/scenario-model/ec2host_test.go test/e2e-framework/AGENTS.md
git commit --no-gpg-sign -m "refactor(scenario/ec2host): defaulted NewParams(), drop NewEC2HostParams + zero-value warning"
```

---

## Task 4: `EnvResolver` + `DispatchAction`; `RunAction` delegates

**Files:**
- Create: `test/e2e-framework/scenario/dispatch.go`
- Modify: `test/e2e-framework/scenario/runnable.go` (`RunAction` delegates)
- Test: `test/e2e-framework/scenario/dispatch_test.go`

**Interfaces:**
- Consumes: `Scenario[Env]`, `Action[Env]`, `LoadProvisionedStack`, `ErrNoProvisionedStack`, `fromRawMessage`, `standalone.HydrateFromResources` (existing).
- Produces:
  - `type EnvResolver[Env any] interface { Resolve(ctx common.Context, stack string) (*Env, error) }`
  - `func DispatchAction[Env any](ctx common.Context, s Scenario[Env], stack, action string, cfg map[string]string, resolver EnvResolver[Env]) error` — looks up the action, decodes its params from `cfg`, resolves the env via `resolver`, runs the handler.
  - `type StateResolver[Env any] struct{}` implementing `EnvResolver[Env]` — loads the provisioned-stack record and hydrates from cached resources+keys (the CLI's behavior). Returns a clear error on `ErrNoProvisionedStack`.

- [ ] **Step 1: Write the failing test** (fake env + fixed resolver — no cloud)

```go
// test/e2e-framework/scenario/dispatch_test.go
package scenario

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

type dispEnv struct{ ran string }

type dispActionParams struct {
	Msg string `scenario:"name=msg,required"`
}

// fixedResolver returns a preset env, ignoring ctx/stack.
type fixedResolver[Env any] struct{ env *Env }

func (r fixedResolver[Env]) Resolve(common.Context, string) (*Env, error) { return r.env, nil }

func dispScenario() Scenario[dispEnv] {
	return Scenario[dispEnv]{
		Name: "disp",
		Actions: map[string]Action[dispEnv]{
			"ping": {
				NewParams: func() any { return &dispActionParams{} },
				Run: func(_ context.Context, e *dispEnv, p any) error {
					e.ran = p.(*dispActionParams).Msg
					return nil
				},
			},
		},
	}
}

func TestDispatchActionRunsHandlerWithDecodedParams(t *testing.T) {
	env := &dispEnv{}
	err := DispatchAction(nil, dispScenario(), "stack", "ping",
		map[string]string{"msg": "hi"}, fixedResolver[dispEnv]{env: env})
	if err != nil {
		t.Fatalf("DispatchAction: %v", err)
	}
	if env.ran != "hi" {
		t.Fatalf("handler not run with decoded params: %q", env.ran)
	}
}

func TestDispatchActionUnknownAction(t *testing.T) {
	if err := DispatchAction(nil, dispScenario(), "stack", "nope", nil, fixedResolver[dispEnv]{env: &dispEnv{}}); err == nil {
		t.Fatal("expected error for unknown action")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestDispatchAction -v`
Expected: FAIL — `undefined: DispatchAction`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/dispatch.go
// <license header>

package scenario

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// EnvResolver produces the typed environment an action runs against. The CLI
// resolves from local state; a test can resolve to a live suite env.
type EnvResolver[Env any] interface {
	Resolve(ctx common.Context, stack string) (*Env, error)
}

// DispatchAction is the single action code path: decode the action's params,
// resolve the environment, run the handler. Only env resolution is pluggable.
func DispatchAction[Env any](ctx common.Context, s Scenario[Env], stack, action string, cfg map[string]string, resolver EnvResolver[Env]) error {
	a, ok := s.Actions[action]
	if !ok {
		return fmt.Errorf("unknown action %q for scenario %q", action, s.Name)
	}
	var ap any
	if a.NewParams != nil {
		ap = a.NewParams()
		sc, err := BuildSchema(ap)
		if err != nil {
			return err
		}
		if err := Decode(sc, cfg, ap); err != nil {
			return err
		}
	}
	env, err := resolver.Resolve(ctx, stack)
	if err != nil {
		return err
	}
	return a.Run(context.Background(), env, ap)
}

// StateResolver hydrates the env from the local provisioned-stack record
// (cached outputs + import keys), with no Pulumi call. This is the CLI default.
type StateResolver[Env any] struct{}

func (StateResolver[Env]) Resolve(ctx common.Context, stack string) (*Env, error) {
	ps, err := LoadProvisionedStack(stack)
	if errors.Is(err, ErrNoProvisionedStack) {
		return nil, fmt.Errorf("no local record for stack %q; actions require a stack created via 'scenariorun create'", stack)
	}
	if err != nil {
		return nil, fmt.Errorf("load provisioned stack state: %w", err)
	}
	return standalone.HydrateFromResources[Env](ctx, fromRawMessage(ps.Resources), ps.Keys)
}
```

Then replace `genericRunnable[Env].RunAction` in `runnable.go` with a delegation:
```go
func (g genericRunnable[Env]) RunAction(ctx common.Context, stack, action string, cfg map[string]string) error {
	return DispatchAction(ctx, g.s, stack, action, cfg, StateResolver[Env]{})
}
```
Remove now-unused imports from `runnable.go` (`errors`, `context`, `standalone` may become unused there — let the compiler guide you; `time`/`environments`/`provisioners` are still used by `Create`).

- [ ] **Step 4: Run tests + full package**

Run: `cd test/e2e-framework && go test ./scenario/ -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/dispatch.go test/e2e-framework/scenario/runnable.go test/e2e-framework/scenario/dispatch_test.go
git commit --no-gpg-sign -m "feat(scenario): EnvResolver + DispatchAction; RunAction delegates via StateResolver"
```

---

## Task 5: Registry-keyed `Create`/`Action`/`Destroy`; CLI delegates

**Files:**
- Create: `test/e2e-framework/scenario/lifecycle.go`
- Modify: `test/e2e-framework/cmd/scenariorun/main.go` (handlers call the package API)
- Test: `test/e2e-framework/scenario/lifecycle_registry_test.go`

**Interfaces:**
- Consumes: `Lookup` (existing).
- Produces:
  - `func Create(ctx common.Context, scenarioName, stack string, cfg map[string]string) error`
  - `func Action(ctx common.Context, scenarioName, stack, action string, cfg map[string]string) error`
  - `func Destroy(ctx common.Context, scenarioName, stack string) error`
  Each resolves the `Runnable` by name and calls it; unknown scenario → clear error.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/lifecycle_registry_test.go
package scenario

import "testing"

func TestLifecycleUnknownScenario(t *testing.T) {
	resetRegistry()
	if err := Create(nil, "ghost", "s", nil); err == nil {
		t.Fatal("Create: expected unknown-scenario error")
	}
	if err := Action(nil, "ghost", "s", "a", nil); err == nil {
		t.Fatal("Action: expected unknown-scenario error")
	}
	if err := Destroy(nil, "ghost", "s"); err == nil {
		t.Fatal("Destroy: expected unknown-scenario error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestLifecycleUnknownScenario -v`
Expected: FAIL — `undefined: Create` (as package function).

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/lifecycle.go
// <license header>

package scenario

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

func lookupOrErr(name string) (Runnable, error) {
	r, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
	return r, nil
}

// Create provisions the named scenario into stack with the given config.
func Create(ctx common.Context, scenarioName, stack string, cfg map[string]string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.Create(ctx, stack, cfg)
}

// Action runs a named action on a running stack.
func Action(ctx common.Context, scenarioName, stack, action string, cfg map[string]string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.RunAction(ctx, stack, action, cfg)
}

// Destroy tears down a running stack.
func Destroy(ctx common.Context, scenarioName, stack string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.Destroy(ctx, stack)
}
```

- [ ] **Step 4: Delegate the CLI handlers**

In `cmd/scenariorun/main.go`, the `create`/`action`/`destroy` `RunE` bodies currently call `r.Create`/`r.RunAction`/`r.Destroy` on the looked-up `Runnable`. Replace those calls with the package API, e.g. inside `createCmd`'s subcommand `RunE`:
```go
cfg := CollectFlags(sc, cmd.Flags())
stack, _ := cmd.Flags().GetString("stack")
return scenario.Create(newCtx(), r.Name(), stack, cfg)
```
and analogously `scenario.Action(newCtx(), r.Name(), stack, name, cfg)` and `scenario.Destroy(newCtx(), r.Name(), stack)`. (Functionally identical; this makes the package API the single entry point.)

- [ ] **Step 5: Test + build + smoke**

Run:
```bash
cd test/e2e-framework && go test ./scenario/ -run TestLifecycleUnknownScenario -v && go build ./cmd/scenariorun && ./scenariorun describe --json | head -3
```
Expected: PASS; build clean; describe still works. (Remove the stray `./scenariorun` binary after: `rm -f scenariorun`.)

- [ ] **Step 6: Commit**

```bash
git add test/e2e-framework/scenario/lifecycle.go test/e2e-framework/cmd/scenariorun/main.go test/e2e-framework/scenario/lifecycle_registry_test.go
git commit --no-gpg-sign -m "feat(scenario): registry-keyed Create/Action/Destroy; CLI delegates to them"
```

---

## Task 6: `scenariotest` e2e bridge

**Files:**
- Create: `test/e2e-framework/testing/scenariotest/scenariotest.go`
- Test: `test/e2e-framework/testing/scenariotest/scenariotest_test.go`

**Interfaces:**
- Consumes: `scenario.Scenario[Env]`, `scenario.DispatchAction`, `scenario.EnvResolver` (Task 4); `e2e.WithProvisioner`, `e2e.SuiteOption`; `standalone.NewContext`.
- Produces:
  - `func WithScenario[Env any](s scenario.Scenario[Env], params any) e2e.SuiteOption` — builds the scenario's provisioner from `params` and wraps it as `e2e.WithProvisioner`. Panics with a clear message if `s.Provisioner` errors (test setup error).
  - `func RunAction[Env any](env *Env, s scenario.Scenario[Env], action string, config map[string]string) error` — runs the action against `env` via `DispatchAction` with a fixed-env resolver (no state, no Pulumi).

- [ ] **Step 1: Write the failing test** (no cloud — WithScenario is compile/adapter-level; RunAction against a fake env)

```go
// test/e2e-framework/testing/scenariotest/scenariotest_test.go
package scenariotest

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

type bridgeEnv struct{ ran bool }

func bridgeScenario() scenario.Scenario[bridgeEnv] {
	return scenario.Scenario[bridgeEnv]{
		Name: "bridge",
		Actions: map[string]scenario.Action[bridgeEnv]{
			"go": {Run: func(_ context.Context, e *bridgeEnv, _ any) error { e.ran = true; return nil }},
		},
	}
}

func TestRunActionAgainstProvidedEnv(t *testing.T) {
	env := &bridgeEnv{}
	if err := RunAction(env, bridgeScenario(), "go", nil); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if !env.ran {
		t.Fatal("action did not run against provided env")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./testing/scenariotest/ -v`
Expected: FAIL — `undefined: RunAction`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/testing/scenariotest/scenariotest.go
// <license header>

// Package scenariotest bridges a registered scenario into an e2e BaseSuite test:
// provision via the scenario's own provisioner, and run its actions against the
// live suite env. Kept separate from the core scenario package so that package
// takes no test-harness dependency.
package scenariotest

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// WithScenario provisions the scenario with params as a BaseSuite provisioner.
// No CLI state is written — the suite owns the env lifecycle.
func WithScenario[Env any](s scenario.Scenario[Env], params any) e2e.SuiteOption {
	prov, err := s.Provisioner(params)
	if err != nil {
		panic(fmt.Sprintf("scenariotest.WithScenario(%q): %v", s.Name, err))
	}
	return e2e.WithProvisioner(prov)
}

// fixedResolver resolves to a preset env (the live suite env), ignoring ctx/stack.
type fixedResolver[Env any] struct{ env *Env }

func (r fixedResolver[Env]) Resolve(common.Context, string) (*Env, error) { return r.env, nil }

// RunAction runs the scenario's action against env via the shared DispatchAction
// (real param decode + handler). Use sparingly — to test that a defined action
// works, not to drive test mutations.
func RunAction[Env any](env *Env, s scenario.Scenario[Env], action string, config map[string]string) error {
	ctx := standalone.NewContext(os.TempDir()) // resolver ignores it
	return scenario.DispatchAction[Env](ctx, s, "", action, config, fixedResolver[Env]{env: env})
}
```

- [ ] **Step 4: Run test + build**

Run: `cd test/e2e-framework && go test ./testing/scenariotest/ -v && go build ./...`
Expected: PASS; build clean.

> Implementer note: confirm `e2e.WithProvisioner` and `e2e.SuiteOption` names/signatures in `testing/e2e/suite_params.go`; adjust if they differ. `provisioners.TypedProvisioner[Env]` satisfies the `provisioners.Provisioner` that `WithProvisioner` expects.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/testing/scenariotest/
git commit --no-gpg-sign -m "feat(scenariotest): WithScenario + RunAction e2e bridge"
```

---

## Task 7: Fake full-lifecycle test (create → state → action → destroy, no cloud)

**Files:**
- Create: `test/e2e-framework/scenario/lifecycle_test.go`

**Interfaces:**
- Consumes: `Register`, `Create`, `Action`, `Destroy`, `LoadProvisionedStack`, `ErrNoProvisionedStack`, `Scenario`/`Action` types, `standalone.NewContext`; `provisioners.TypedProvisioner`/`RawResources`; `components.Importable`.
- Produces: a cloud-free test that a scenario's create writes state (config+resources+keys), an action hydrates from that state and runs against a correctly-imported env, and destroy removes the state.

**This is the linchpin test.** It needs a fake env with a real `Importable` component and a fake provisioner that returns matching `RawResources` and sets the component's key.

- [ ] **Step 1: Write the test**

```go
// test/e2e-framework/scenario/lifecycle_test.go
package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// fakeComp is an importable component whose Import records the value.
type fakeComp struct {
	components.JSONImporter
	Value string `json:"value"`
}

// lifeEnv is a fake environment with one importable component.
type lifeEnv struct {
	Comp *fakeComp
}

const lifeKey = "comp" // export key the fake provisioner assigns

// fakeProvisioner implements TypedProvisioner[lifeEnv]: it sets the component key
// and returns a matching RawResources map (mirrors what a Pulumi Export does).
type fakeProvisioner struct{}

func (fakeProvisioner) ID() string                                          { return "fake" }
func (fakeProvisioner) Destroy(context.Context, string, io.Writer) error    { return nil }
func (fakeProvisioner) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *lifeEnv) (provisioners.RawResources, error) {
	env.Comp.SetKey(lifeKey)
	blob, _ := json.Marshal(map[string]string{"value": "hello"})
	return provisioners.RawResources{lifeKey: blob}, nil
}

func lifeScenario(ran *string) Scenario[lifeEnv] {
	return Scenario[lifeEnv]{
		Name:      "life",
		NewParams: func() any { return &struct{}{} },
		Provisioner: func(any) (provisioners.TypedProvisioner[lifeEnv], error) {
			return fakeProvisioner{}, nil
		},
		Actions: map[string]Action[lifeEnv]{
			"observe": {Run: func(_ context.Context, e *lifeEnv, _ any) error { *ran = e.Comp.Value; return nil }},
		},
	}
}

func TestFullLifecycleNoCloud(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())
	resetRegistry()
	var ran string
	Register(lifeScenario(&ran))
	ctx := standalone.NewContext(t.TempDir())

	// create -> provision + persist state
	if err := Create(ctx, "life", "st1", map[string]string{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ps, err := LoadProvisionedStack("st1")
	if err != nil {
		t.Fatalf("expected state record: %v", err)
	}
	if ps.Scenario != "life" || ps.Keys["Comp"] != lifeKey || len(ps.Resources) == 0 {
		t.Fatalf("state record wrong: %+v", ps)
	}

	// action -> hydrate from cached state (no provisioner), run handler
	if err := Action(ctx, "life", "st1", "observe", nil); err != nil {
		t.Fatalf("Action: %v", err)
	}
	if ran != "hello" {
		t.Fatalf("action saw wrong hydrated value: %q", ran)
	}

	// destroy -> state removed
	if err := Destroy(ctx, "life", "st1"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := LoadProvisionedStack("st1"); !errors.Is(err, ErrNoProvisionedStack) {
		t.Fatalf("expected state removed, got %v", err)
	}
}
```

> Implementer notes:
> - Confirm `environments.ImportKeys` keys the map by **Go field name** (`"Comp"`); the test asserts `ps.Keys["Comp"]`. If `ImportKeys` uses a different key convention, adjust the assertion to match its actual output (read `environments.ImportKeys`).
> - Confirm `provisioners.TypedProvisioner[Env]` method set (`ID`, `Destroy`, `ProvisionEnv`) — mirror `standalone`'s usage. If `standalone.Destroy` needs the provisioner to also satisfy something else, the `fakeProvisioner` above covers `ID`/`Destroy`/`ProvisionEnv`.
> - `standalone.ProvisionWithResources` calls `CreateEnv` (allocates `env.Comp`) then `ProvisionEnv`; the fake sets the key there, matching real `Export` behavior.

- [ ] **Step 2: Run it (RED then GREEN)**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestFullLifecycleNoCloud -v`
Expected: PASS. If it fails on key convention, fix the assertion per the note (this test *is* the verification that create→state→action→destroy works without cloud).

- [ ] **Step 3: Run the whole package**

Run: `cd test/e2e-framework && go test ./scenario/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add test/e2e-framework/scenario/lifecycle_test.go
git commit --no-gpg-sign -m "test(scenario): cloud-free full-lifecycle test (create->state->action->destroy)"
```

---

## Task 8: Refactor the ec2host E2E test onto the bridge

**Files:**
- Modify: `test/new-e2e/tests/scenario-model/ec2host_test.go`

**Interfaces:**
- Consumes: `scenariotest.WithScenario`, `scenariotest.RunAction` (Task 6); `ec2host.Scenario`, `ec2host.NewParams` (Task 3).

- [ ] **Step 1: Rewrite the test to use the bridge**

```go
// test/new-e2e/tests/scenario-model/ec2host_test.go
// <license header>

package scenariomodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/scenariotest"
)

type ec2HostSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestEC2HostScenario(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ec2HostSuite{},
		scenariotest.WithScenario(ec2host.Scenario(), ec2host.NewParams()))
}

func (s *ec2HostSuite) TestRunCommandAction() {
	// exercise the real action decode + dispatch against the live suite env
	err := scenariotest.RunAction(s.Env(), ec2host.Scenario(), "run-command",
		map[string]string{"command": "echo hello"})
	s.Require().NoError(err)
}
```

- [ ] **Step 2: Compile-check (no provisioning)**

Run: `cd test/new-e2e && go vet ./tests/scenario-model/...`
Expected: clean. (Do not run the gated suite here.)

- [ ] **Step 3: Commit**

```bash
git add test/new-e2e/tests/scenario-model/ec2host_test.go
git commit --no-gpg-sign -m "test(scenario): drive ec2host e2e test via scenariotest bridge"
```

---

## Task 9: `agent-health` scenario package (from the existing provisioner)

**Files:**
- Read first: `test/new-e2e/tests/agent-health/provisioner.go`, `docker_permission_test.go`, `fixtures/docker_permission_agent_config.yaml`, `fixtures/docker-compose.busybox.yaml`
- Create: `test/e2e-framework/scenario/scenarios/agenthealth/env.go`
- Create: `test/e2e-framework/scenario/scenarios/agenthealth/params.go`
- Create: `test/e2e-framework/scenario/scenarios/agenthealth/scenario.go`
- Create: `test/e2e-framework/scenario/scenarios/agenthealth/fixtures/` (copy the two fixtures needed by the run-func)
- Modify: `test/e2e-framework/cmd/scenariorun/import_scenarios.go`
- Test: `test/e2e-framework/scenario/scenarios/agenthealth/scenario_test.go`

**Interfaces:**
- Consumes: `scenario` (Scenario/Action/Register/NewParams), `scenario/params` (`AgentParams`, `FakeintakeParams`), `provisioners.NewTypedPulumiProvisioner`, and the framework building blocks used by the existing provisioner (`aws.NewEnvironment`, `ec2.NewVM`/`ec2.WithOSArch`, `fakeintake.NewECSFargateInstance`, `docker.NewAWSManager`/`ComposeStrUp`, `agent.NewHostAgent`, `agentparams.*`, `components.*`, `e2eos`).
- Produces: `agenthealth.Env`, `agenthealth.Params`, `agenthealth.NewParams()`, `agenthealth.Provisioner(*Params)`, `agenthealth.Scenario()`, `agenthealth.Register()`.

- [ ] **Step 1: Read the existing provisioner** to port its logic faithfully.

Run: `sed -n '1,120p' test/new-e2e/tests/agent-health/provisioner.go && sed -n '1,60p' test/new-e2e/tests/agent-health/docker_permission_test.go`

- [ ] **Step 2: Write `env.go`**

```go
// test/e2e-framework/scenario/scenarios/agenthealth/env.go
// <license header>

package agenthealth

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Env is a VM with the host Agent, a dockerized workload, and a fakeintake.
type Env struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

func (e *Env) Init(_ common.Context) error { return nil }
```

- [ ] **Step 3: Write `params.go`**

```go
// test/e2e-framework/scenario/scenarios/agenthealth/params.go
// <license header>

package agenthealth

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params"
)

type Params struct {
	OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
	Arch string `scenario:"name=arch,default=x86_64,help=CPU architecture,enum=x86_64|arm64"`

	Agent      params.AgentParams
	Fakeintake params.FakeintakeParams
}

func NewParams() *Params { return scenario.NewParams[Params]() }
```

- [ ] **Step 4: Write `scenario.go`** — port the run-func from `provisioner.go`, layering CLI agent options after the scenario-intrinsic base config; curated actions only.

```go
// test/e2e-framework/scenario/scenarios/agenthealth/scenario.go
// <license header>

package agenthealth

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/docker_permission_agent_config.yaml
var baseAgentConfig string

//go:embed fixtures/docker-compose.busybox.yaml
var busyboxCompose string

func osArch(p *Params) (e2eos.Descriptor, e2eos.Architecture, error) {
	// Reuse the same mapping ec2host uses; adjust symbol names to the real API.
	var desc e2eos.Descriptor
	switch p.OS {
	case "ubuntu-22.04":
		desc = e2eos.Ubuntu2204
	case "debian-12":
		desc = e2eos.Debian12
	case "amazon-linux-2023":
		desc = e2eos.AmazonLinux2023
	default:
		return e2eos.Descriptor{}, "", fmt.Errorf("unknown os %q", p.OS)
	}
	switch p.Arch {
	case "x86_64", "":
		return desc, e2eos.AMD64Arch, nil
	case "arm64":
		return desc, e2eos.ARM64Arch, nil
	default:
		return e2eos.Descriptor{}, "", fmt.Errorf("unknown arch %q", p.Arch)
	}
}

func provision(p *Params) provisioners.PulumiEnvRunFunc[Env] {
	return func(ctx *pulumi.Context, env *Env) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		desc, arch, err := osArch(p)
		if err != nil {
			return err
		}
		host, err := ec2.NewVM(awsEnv, "agent-health", ec2.WithOSArch(desc, arch))
		if err != nil {
			return err
		}
		if err := host.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}
		fi, err := fakeintake.NewECSFargateInstance(awsEnv, "agent-health")
		if err != nil {
			return err
		}
		if err := fi.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
			return err
		}
		dm, err := docker.NewAWSManager(&awsEnv, host)
		if err != nil {
			return err
		}
		if err := dm.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}
		composeCmd, err := dm.ComposeStrUp("busybox",
			[]docker.ComposeInlineManifest{{Name: "busybox", Content: pulumi.String(busyboxCompose)}}, nil)
		if err != nil {
			return err
		}
		agentOpts := []agentparams.Option{
			agentparams.WithAgentConfig(baseAgentConfig),
			agentparams.WithFakeintake(fi),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeCmd})),
		}
		userOpts, err := p.Agent.ToOptions()
		if err != nil {
			return err
		}
		agentOpts = append(agentOpts, userOpts...)
		ag, err := agent.NewHostAgent(&awsEnv, host, agentOpts...)
		if err != nil {
			return err
		}
		return ag.Export(ctx, &env.Agent.HostAgentOutput)
	}
}

// Provisioner adapts params to a typed provisioner for the custom Env.
func Provisioner(p *Params) (provisioners.TypedProvisioner[Env], error) {
	return provisioners.NewTypedPulumiProvisioner("agent-health", provision(p), nil), nil
}

func Scenario() scenario.Scenario[Env] {
	return scenario.Scenario[Env]{
		Name:        "agent-health",
		Description: "VM with host Agent + dockerized workload reporting to fakeintake",
		NewParams:   func() any { return NewParams() },
		Provisioner: func(a any) (provisioners.TypedProvisioner[Env], error) { return Provisioner(a.(*Params)) },
		Actions: map[string]scenario.Action[Env]{
			"connection-info": {
				Description: "Print SSH connection details for the VM",
				Run: func(_ context.Context, e *Env, _ any) error {
					h := e.RemoteHost.HostOutput
					fmt.Printf("ssh %s@%s -p %d\n", h.Username, h.Address, h.Port)
					return nil
				},
			},
			"restart-agent": {
				Description: "Restart the Datadog Agent",
				Run: func(_ context.Context, e *Env, _ any) error { return e.Agent.Client.Restart() },
			},
		},
	}
}

func Register() { scenario.Register(Scenario()) }
```

> Implementer notes (verify against source; port faithfully from `provisioner.go`):
> - Copy the two fixtures the run-func embeds into `agenthealth/fixtures/` (from `test/new-e2e/tests/agent-health/fixtures/`).
> - Confirm exact symbols: `e2eos.Descriptor`/`Ubuntu2204`/`Debian12`/`AmazonLinux2023`/`AMD64Arch`/`ARM64Arch`, `ec2.NewVM`/`ec2.WithOSArch`, `fakeintake.NewECSFargateInstance`, `docker.NewAWSManager`/`ComposeStrUp`/`ComposeInlineManifest`, `agent.NewHostAgent`, `agentparams.WithAgentConfig`/`WithFakeintake`/`WithPulumiResourceOptions`, `e.Agent.Client.Restart()`, `components.RemoteHost/RemoteHostAgent/FakeIntake/RemoteHostDocker` and their `*Output` fields. The existing `provisioner.go` is the ground truth — mirror its calls.
> - `env.RemoteHost`/`Agent`/`Fakeintake`/`Docker` are non-nil in the run-func (allocated by `CreateEnv`), so `&env.RemoteHost.HostOutput` etc. are safe.

- [ ] **Step 5: Register + write unit test**

Add to `cmd/scenariorun/import_scenarios.go`:
```go
import "github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/agenthealth"
// in registerScenarios():
agenthealth.Register()
```

```go
// test/e2e-framework/scenario/scenarios/agenthealth/scenario_test.go
package agenthealth

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

func TestSchemaExposesAgentAndOSFlags(t *testing.T) {
	sc, err := scenario.BuildSchema(NewParams())
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	names := map[string]bool{}
	for _, f := range sc.Fields {
		names[f.Name] = true
	}
	for _, want := range []string{"os", "arch", "agent-version", "use-fakeintake"} {
		if !names[want] {
			t.Errorf("missing flag %q (got %v)", want, names)
		}
	}
}

func TestProvisionerBuilds(t *testing.T) {
	prov, err := Provisioner(NewParams())
	if err != nil || prov == nil {
		t.Fatalf("Provisioner: prov=%v err=%v", prov, err)
	}
}

func TestActionsRegistered(t *testing.T) {
	a := Scenario().Actions
	if _, ok := a["connection-info"]; !ok {
		t.Error("connection-info action missing")
	}
	if _, ok := a["restart-agent"]; !ok {
		t.Error("restart-agent action missing")
	}
}
```

- [ ] **Step 6: Build + test + describe smoke**

Run:
```bash
cd test/e2e-framework && go build ./... && go test ./scenario/scenarios/agenthealth/ -v
go build -o /tmp/scenariorun ./cmd/scenariorun && /tmp/scenariorun describe --json | grep -q '"agent-health"' && echo OK && rm -f /tmp/scenariorun
```
Expected: tests PASS; `agent-health` present in describe output.

- [ ] **Step 7: Commit**

```bash
git add test/e2e-framework/scenario/scenarios/agenthealth/ test/e2e-framework/cmd/scenariorun/import_scenarios.go
git commit --no-gpg-sign -m "feat(scenario): agent-health scenario (VM + host agent + docker app + fakeintake)"
```

---

## Task 10: Swap the docker-permission suite onto the scenario

**Files:**
- Modify: `test/new-e2e/tests/agent-health/docker_permission_test.go`
- Possibly remove: `test/new-e2e/tests/agent-health/provisioner.go` (if only the docker-permission suite used it — verify first)

**Interfaces:**
- Consumes: `agenthealth.Scenario`, `agenthealth.NewParams` (Task 9); `scenariotest.WithScenario`.

- [ ] **Step 1: Verify what uses `provisioner.go`**

Run: `grep -rn "dockerPermissionEnvProvisioner\|dockerPermissionEnv" test/new-e2e/tests/agent-health/`
If only `docker_permission_test.go` references them, `provisioner.go` and the local `dockerPermissionEnv` struct can be deleted after the swap. If others use it, leave it.

- [ ] **Step 2: Swap the provisioning line and env type**

In `docker_permission_test.go`:
- Change the suite embed from `e2e.BaseSuite[dockerPermissionEnv]` to `e2e.BaseSuite[agenthealth.Env]`.
- Change the provisioning in `TestDockerPermissionSuite` from
  `e2e.Run(t, &dockerPermissionSuite{}, e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil))`
  to
  `e2e.Run(t, &dockerPermissionSuite{}, scenariotest.WithScenario(agenthealth.Scenario(), agenthealth.NewParams()))`.
- Update field access if the local env used different field names (e.g. `s.Env().Fakeintake` vs `s.Env().FakeIntake`) to match `agenthealth.Env` (`RemoteHost`, `Agent`, `Fakeintake`, `Docker`).
- **Leave all subtest logic and inline mutations unchanged** (`s.Env().RemoteHost.MustExecute("sudo chmod …")`, `s.Env().Agent.Client.Restart()`, `s.Env().Fakeintake.Client()...`). Do NOT convert them to actions.
- Add the import for `agenthealth` and `scenariotest`; remove the now-unused custom-provisioner import if `provisioner.go` is deleted.

- [ ] **Step 3: Compile-check**

Run: `cd test/new-e2e && go vet ./tests/agent-health/...`
Expected: clean. (Gated suite is not run here; it provisions real AWS.)

- [ ] **Step 4: Commit**

```bash
git add test/new-e2e/tests/agent-health/
git commit --no-gpg-sign -m "test(agent-health): drive docker-permission suite via the agent-health scenario"
```

---

## Task 11: Documentation

**Files:**
- Modify: `test/e2e-framework/AGENTS.md`

- [ ] **Step 1: Update the "Unified scenario model" section**

Add/adjust so it states:
- `scenario.NewParams[T]()` and each scenario's `NewParams()` are the blessed defaulted constructors; CLI `Decode` applies the same tag defaults (no zero-value trap).
- Action execution is one path (`DispatchAction`) with pluggable `EnvResolver` (CLI = state; tests = suite env via `scenariotest.RunAction`).
- Tests reuse a scenario with `scenariotest.WithScenario(sc, sc.NewParams())`; subtests use `s.Env()` as usual.
- **Actions are curated CLI affordances, not test-step mirrors** — test mutations stay as Go against `s.Env()`.
- `agent-health` is a worked custom-env example (VM + host agent + docker app + fakeintake).

- [ ] **Step 2: Commit**

```bash
git add test/e2e-framework/AGENTS.md
git commit --no-gpg-sign -m "docs(scenario): document defaults, single action path, and curated-actions principle"
```

---

## Self-Review

**1. Spec coverage:**
- Phase 1 (ApplyDefaults, NewParams[T], Decode overlay, ec2host defaulted, drop warning) → Tasks 1, 2, 3. ✓
- Phase 2 (registry-keyed Create/Action/Destroy, EnvResolver+DispatchAction, RunAction delegates, scenariotest bridge) → Tasks 4, 5, 6. ✓
- Phase 3 (fake full-lifecycle test, suite-resolver action test, ec2host test refactor) → Tasks 7 (full lifecycle), 6-test/8 (suite-resolver via scenariotest.RunAction), 8 (ec2host refactor). ✓
- Applied example agent-health (scenario package, register, docker-permission swap, curated actions) → Tasks 9, 10. ✓
- Define-once guarantee + agent config from CLI → embodied in Tasks 6/9 (WithScenario + AgentParams flags). ✓
- Docs → Task 11. ✓

**2. Placeholder scan:** No TBD/TODO. Codebase-dependent specifics (agent-health `provisioner.go` port, exact `e2eos`/`ec2`/`agent` symbol names, `ImportKeys` key convention, `e2e.WithProvisioner` name) are flagged as explicit "verify against source" implementer notes with the ground-truth file named — not vague placeholders.

**3. Type consistency:** `EnvResolver[Env].Resolve(common.Context, string) (*Env, error)` consistent across Task 4 (`StateResolver`) and Task 6 (`fixedResolver`). `DispatchAction[Env](common.Context, Scenario[Env], stack, action string, map[string]string, EnvResolver[Env])` consistent between Tasks 4, 6. `NewParams[T]()` (Task 1) used by Tasks 3, 9. Registry API `Create/Action/Destroy(common.Context, name, stack[, action], cfg)` consistent between Tasks 5, 7. `agenthealth.Env` fields (`RemoteHost/Agent/Fakeintake/Docker`) consistent between Tasks 9, 10.

**Known verification points for the implementer** (flagged inline, not gaps): the agent-health `provisioner.go` logic to port; exact framework symbol names (`e2eos.*`, `ec2.WithOSArch`, `docker.*`, `agent.NewHostAgent`, `agentparams.*`, `e.Agent.Client.Restart`); `environments.ImportKeys` key convention; `e2e.WithProvisioner`/`SuiteOption`; whether `provisioner.go` is deletable after the swap.
