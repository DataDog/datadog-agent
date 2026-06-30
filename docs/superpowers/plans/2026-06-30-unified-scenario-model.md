# Unified Scenario Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Define E2E scenarios once in Go (canonical tagged params + reusable param components + provisioner + actions) and drive that single definition from E2E tests, a dynamic Go CLI fronted by `dda`, and a per-commit long-running service stub.

**Architecture:** A new `scenario` package in the `test/e2e-framework` Go module provides a reflection layer (tagged struct ⇄ schema ⇄ flags + decode/validate), a generic `Scenario[Env]` authoring type, and a type-erased `Runnable` registry. Reusable param components (`AgentParams`, `FakeintakeParams`) embed into a scenario's canonical struct and convert to existing `agentparams.Option`/`fakeintake.Option`. Provisioning has two convergent paths: Go tests keep the existing typed `With…` options and the existing provisioner unchanged (zero migration), while the CLI/service decode flags into the tagged struct and map them onto those same typed options and provisioner. A cobra CLI (`scenariorun`) builds its command tree from the registry by reflection; `dda lab` forwards to it. A service stub builds-and-drives the `scenariorun` binary from a caller-specified commit via its stable `describe`/`create`/`action`/`destroy` protocol. The AWS EC2 host scenario is the reference implementation.

**Tech Stack:** Go (module `github.com/DataDog/datadog-agent/test/e2e-framework`), `spf13/cobra` + `spf13/pflag` (already vendored, indirect → promote to direct), Pulumi provisioning via the existing `testing/standalone` driver, Python `invoke`/`dda` for the forwarder.

## Global Constraints

- **Go module:** All Go code in this plan lives in the **`test/e2e-framework`** module (separate `go.mod`). Run Go commands from inside `test/e2e-framework/`.
- **Build/test idiom for this module:** This module is tag-light; the established precedent (`tasks/ai_sandbox.py:125-126`) builds its binaries with a plain `go build` run inside the module directory. Unit tests for pure packages added here run with `go test` inside the module (e.g. `cd test/e2e-framework && go test ./scenario/... -v`). This is the one documented exception to the repo-wide "never raw go" rule, which targets the main tagged module.
- **License header:** Every new `.go` file starts with the 4-line Apache header used across the repo:
  ```go
  // Unless explicitly stated otherwise all files in this repository are licensed
  // under the Apache License Version 2.0.
  // This product includes software developed at Datadog (https://www.datadoghq.com/).
  // Copyright 2025-present Datadog, Inc.
  ```
- **`scenario` tag grammar:** comma-separated `key=value` pairs in a `scenario:"…"` struct tag. Keys: `name`, `default`, `help`, `enum` (pipe-separated values), `required` (bare key = true). A lone `-` (`scenario:"-"`) means "not introspectable — Go-only escape hatch; skip in schema/flags". **Help text and defaults must not contain commas** (parser splits on commas); document this on the tag helper.
- **Protocol version:** the CLI's `describe --json` output includes `"protocolVersion": 1`. Bump only on breaking protocol changes.
- **No real cloud in unit tests:** unit tests must never call real Pulumi/AWS. Use the in-memory fakes defined in Task 4.
- **Naming:** CLI binary = `scenariorun`; user-facing command = `dda lab` (forwarder); service binary = `scenario-service`.
- **Deviation from spec (documented):** the spec tentatively located the CLI at `test/new-e2e/run` ("evolving PR #51650"). That PR is unmerged and used an untyped `pulumi.RunFunc` registry. This plan instead places the binary at `test/e2e-framework/cmd/scenariorun` (same module as everything it imports, beside `cmd/ai-sandbox`), wrapped by a `dda` task mirroring `tasks/ai_sandbox.py`.

---

## File Structure

**New package — reflection + scenario model** (`test/e2e-framework/scenario/`)
- `schema.go` — `Kind`, `Field`, `Schema`, tag parsing, `BuildSchema`.
- `decode.go` — `Decode` (map[string]string → struct, defaults, validation).
- `flags.go` — `RegisterFlags`, `CollectFlags` (pflag ⇄ schema).
- `scenario.go` — generic `Scenario[Env]`, `Action[Env]`.
- `runnable.go` — type-erased `Runnable` interface + `genericRunnable[Env]` adapter (drives `standalone`).
- `registry.go` — package registry: `Register[Env]`, `Lookup`, `List`, `reset` (test helper).
- `describe.go` — `Description` struct + `Describe()` (JSON-serializable, includes `protocolVersion`).

**Reusable param components** (`test/e2e-framework/scenario/params/`)
- `agent.go` — `AgentParams` + `ToOptions()`.
- `fakeintake.go` — `FakeintakeParams` + `ToOptions()`.

**Reference scenario** (`test/e2e-framework/scenario/scenarios/ec2host/`)
- `params.go` — `EC2HostParams` (embeds components) + hand-written escape-hatch option helpers.
- `scenario.go` — provisioner adapter (→ `awshost.Provisioner`), actions, `Register`.

**CLI** (`test/e2e-framework/cmd/scenariorun/`)
- `main.go` — cobra root + dynamic command tree from the registry; `serve` delegates to service.
- `import_scenarios.go` — blank-imports scenario packages to register them.

**Service stub** (`test/e2e-framework/cmd/scenario-service/`)
- `main.go` — HTTP server.
- `builder.go` — per-commit checkout + build + drive of `scenariorun`.

**Python / docs**
- `tasks/scenario.py` — `dda lab` forwarder (build-if-needed + passthrough), mirrors `tasks/ai_sandbox.py`.
- `tasks/__init__.py` — register the new collection.
- `test/e2e-framework/AGENTS.md` — document the scenario model.

**E2E test** (`test/new-e2e/`)
- `tests/scenario-model/ec2host_test.go` — drives the reference scenario via its provisioner + invokes an action handler.

---

## Task 1: Reflection — tag parsing and schema

**Files:**
- Create: `test/e2e-framework/scenario/schema.go`
- Test: `test/e2e-framework/scenario/schema_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Kind string` with `KindString="string"`, `KindBool="bool"`, `KindInt="int"`.
  - `type Field struct { Name, GoName string; Kind Kind; Default, Help string; Enum []string; Required bool; Index []int }`
  - `type Schema struct { Fields []Field }`
  - `func BuildSchema(v any) (Schema, error)` — `v` is a pointer to a struct; recurses into embedded/nested struct fields; skips `scenario:"-"`; errors on unsupported field kinds that are tagged.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/schema_test.go
package scenario

import "testing"

type compChild struct {
	Flavor string `scenario:"name=agent-flavor,enum=datadog-agent|datadog-fips-agent"`
}

type schemaSample struct {
	OS       string    `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12"`
	Replicas int       `scenario:"name=replicas,default=1"`
	Verbose  bool      `scenario:"name=verbose"`
	Required string    `scenario:"name=token,required"`
	Child    compChild // embedded component, recurse
	Hidden   []string  `scenario:"-"`            // escape hatch, skipped
	Untagged string                              // no tag, skipped
}

func TestBuildSchema(t *testing.T) {
	s, err := BuildSchema(&schemaSample{})
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	byName := map[string]Field{}
	for _, f := range s.Fields {
		byName[f.Name] = f
	}
	if len(byName) != 5 {
		t.Fatalf("want 5 fields, got %d (%v)", len(byName), s.Fields)
	}
	os := byName["os"]
	if os.Kind != KindString || os.Default != "ubuntu-22.04" || os.Help != "Operating system" {
		t.Errorf("os field wrong: %+v", os)
	}
	if len(os.Enum) != 2 || os.Enum[0] != "ubuntu-22.04" {
		t.Errorf("os enum wrong: %v", os.Enum)
	}
	if byName["replicas"].Kind != KindInt {
		t.Errorf("replicas kind wrong: %v", byName["replicas"].Kind)
	}
	if byName["verbose"].Kind != KindBool {
		t.Errorf("verbose kind wrong: %v", byName["verbose"].Kind)
	}
	if !byName["token"].Required {
		t.Errorf("token should be required")
	}
	if _, ok := byName["agent-flavor"]; !ok {
		t.Errorf("nested component field agent-flavor missing")
	}
	if _, ok := byName["Hidden"]; ok {
		t.Errorf("escape-hatch field must be skipped")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestBuildSchema -v`
Expected: FAIL — `undefined: BuildSchema` (build error).

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/schema.go
// <license header>

// Package scenario provides a reflection-based model for defining e2e scenarios
// once and driving them from tests, a CLI, and a service.
package scenario

import (
	"fmt"
	"reflect"
	"strings"
)

// Kind is the supported flag/field kind.
type Kind string

const (
	KindString Kind = "string"
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
)

// Field is one introspectable scenario parameter.
type Field struct {
	Name     string // CLI flag / config key
	GoName   string // Go struct field name
	Kind     Kind
	Default  string
	Help     string
	Enum     []string
	Required bool
	Index    []int // reflect field index path (supports nested components)
}

// Schema is the ordered set of a struct's introspectable fields.
type Schema struct {
	Fields []Field
}

// BuildSchema reflects a pointer-to-struct into a Schema, recursing into nested
// struct fields (reusable param components) and skipping `scenario:"-"` and
// untagged fields.
func BuildSchema(v any) (Schema, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return Schema{}, fmt.Errorf("BuildSchema: want pointer to struct, got %T", v)
	}
	var s Schema
	if err := walk(rv.Elem().Type(), nil, &s); err != nil {
		return Schema{}, err
	}
	return s, nil
}

func walk(t reflect.Type, prefix []int, s *Schema) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		idx := append(append([]int{}, prefix...), i)
		tag, hasTag := sf.Tag.Lookup("scenario")
		if hasTag && strings.TrimSpace(tag) == "-" {
			continue // escape hatch
		}
		// Recurse into struct-typed fields (reusable components).
		if sf.Type.Kind() == reflect.Struct {
			if err := walk(sf.Type, idx, s); err != nil {
				return err
			}
			continue
		}
		if !hasTag {
			continue // not introspectable
		}
		info := parseTag(tag)
		kind, err := kindOf(sf.Type)
		if err != nil {
			return fmt.Errorf("field %s: %w", sf.Name, err)
		}
		s.Fields = append(s.Fields, Field{
			Name:     info.name,
			GoName:   sf.Name,
			Kind:     kind,
			Default:  info.def,
			Help:     info.help,
			Enum:     info.enum,
			Required: info.required,
			Index:    idx,
		})
	}
	return nil
}

func kindOf(t reflect.Type) (Kind, error) {
	switch t.Kind() {
	case reflect.String:
		return KindString, nil
	case reflect.Bool:
		return KindBool, nil
	case reflect.Int, reflect.Int64:
		return KindInt, nil
	default:
		return "", fmt.Errorf("unsupported tagged field kind %s", t.Kind())
	}
}

type tagInfo struct {
	name     string
	def      string
	help     string
	enum     []string
	required bool
}

func parseTag(tag string) tagInfo {
	var info tagInfo
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, hasEq := strings.Cut(part, "=")
		switch strings.TrimSpace(key) {
		case "name":
			info.name = val
		case "default":
			info.def = val
		case "help":
			info.help = val
		case "enum":
			if hasEq && val != "" {
				info.enum = strings.Split(val, "|")
			}
		case "required":
			info.required = true
		}
	}
	return info
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestBuildSchema -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/schema.go test/e2e-framework/scenario/schema_test.go
git commit --no-gpg-sign -m "feat(scenario): reflect tagged structs into a schema"
```

---

## Task 2: Reflection — decode and validate

**Files:**
- Create: `test/e2e-framework/scenario/decode.go`
- Test: `test/e2e-framework/scenario/decode_test.go`

**Interfaces:**
- Consumes: `Schema`, `Field`, `Kind` (Task 1).
- Produces:
  - `func Decode(s Schema, values map[string]string, target any) error` — `target` is a pointer to the same struct type the schema was built from. Applies `Default` when a key is absent; validates `Required`; validates `Enum`; converts `bool`/`int`; sets fields via `Index`. Unknown keys in `values` → error.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/decode_test.go
package scenario

import "testing"

func TestDecodeAppliesDefaultsAndValidates(t *testing.T) {
	var got schemaSample
	s, _ := BuildSchema(&schemaSample{})

	// token is required; supply it. os omitted -> default. verbose true.
	err := Decode(s, map[string]string{"token": "abc", "verbose": "true", "replicas": "3"}, &got)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "ubuntu-22.04" {
		t.Errorf("default not applied, OS=%q", got.OS)
	}
	if got.Required != "abc" || !got.Verbose || got.Replicas != 3 {
		t.Errorf("decoded wrong: %+v", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	s, _ := BuildSchema(&schemaSample{})
	cases := map[string]map[string]string{
		"missing required": {"os": "debian-12"},
		"bad enum":         {"token": "x", "os": "windows"},
		"unknown key":      {"token": "x", "nope": "1"},
		"bad int":          {"token": "x", "replicas": "abc"},
	}
	for name, vals := range cases {
		var dst schemaSample
		if err := Decode(s, vals, &dst); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestDecode -v`
Expected: FAIL — `undefined: Decode`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/decode.go
// <license header>

package scenario

import (
	"fmt"
	"reflect"
	"strconv"
)

// Decode populates target (pointer to the struct the schema came from) from a
// map of string values, applying defaults and validating required/enum/kind.
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
	elem := rv.Elem()
	for _, f := range s.Fields {
		raw, present := values[f.Name]
		if !present {
			if f.Required {
				return fmt.Errorf("missing required option %q", f.Name)
			}
			raw = f.Default
			if raw == "" {
				continue // leave zero value
			}
		}
		if len(f.Enum) > 0 && !contains(f.Enum, raw) {
			return fmt.Errorf("option %q: %q not in [%v]", f.Name, raw, f.Enum)
		}
		fv := elem.FieldByIndex(f.Index)
		if err := setValue(fv, f.Kind, raw); err != nil {
			return fmt.Errorf("option %q: %w", f.Name, err)
		}
	}
	return nil
}

func setValue(fv reflect.Value, kind Kind, raw string) error {
	switch kind {
	case KindString:
		fv.SetString(raw)
	case KindBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid bool %q", raw)
		}
		fv.SetBool(b)
	case KindInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int %q", raw)
		}
		fv.SetInt(n)
	default:
		return fmt.Errorf("unsupported kind %s", kind)
	}
	return nil
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestDecode -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/decode.go test/e2e-framework/scenario/decode_test.go
git commit --no-gpg-sign -m "feat(scenario): decode and validate option maps into structs"
```

---

## Task 3: Reflection — pflag binding

**Files:**
- Create: `test/e2e-framework/scenario/flags.go`
- Test: `test/e2e-framework/scenario/flags_test.go`

**Interfaces:**
- Consumes: `Schema`, `Field`, `Kind` (Task 1); `github.com/spf13/pflag`.
- Produces:
  - `func RegisterFlags(s Schema, fs *pflag.FlagSet)` — one flag per field (typed: string/bool/int), default + help (help annotated with enum values) from the field.
  - `func CollectFlags(s Schema, fs *pflag.FlagSet) map[string]string` — returns only flags the user explicitly **changed** (so `Decode` can apply struct defaults uniformly), as stringified values.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/flags_test.go
package scenario

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestRegisterAndCollectFlags(t *testing.T) {
	s, _ := BuildSchema(&schemaSample{})
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	RegisterFlags(s, fs)

	if err := fs.Parse([]string{"--token", "abc", "--verbose", "--replicas", "5"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := CollectFlags(s, fs)
	if got["token"] != "abc" || got["verbose"] != "true" || got["replicas"] != "5" {
		t.Fatalf("collected wrong: %v", got)
	}
	// os was not set on the command line -> must not appear (Decode applies the default).
	if _, ok := got["os"]; ok {
		t.Errorf("unchanged flag os should not be collected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestRegisterAndCollectFlags -v`
Expected: FAIL — `undefined: RegisterFlags`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/flags.go
// <license header>

package scenario

import (
	"strings"

	"github.com/spf13/pflag"
)

// RegisterFlags registers one pflag per schema field on fs.
func RegisterFlags(s Schema, fs *pflag.FlagSet) {
	for _, f := range s.Fields {
		help := f.Help
		if len(f.Enum) > 0 {
			help = strings.TrimSpace(help + " (one of: " + strings.Join(f.Enum, ", ") + ")")
		}
		switch f.Kind {
		case KindBool:
			fs.Bool(f.Name, f.Default == "true", help)
		case KindInt:
			def := 0
			// best-effort default; Decode re-applies the canonical default too.
			if f.Default != "" {
				_, _ = fmtSscan(f.Default, &def)
			}
			fs.Int(f.Name, def, help)
		default:
			fs.String(f.Name, f.Default, help)
		}
	}
}

// CollectFlags returns only flags the user explicitly changed, stringified.
func CollectFlags(s Schema, fs *pflag.FlagSet) map[string]string {
	out := map[string]string{}
	for _, f := range s.Fields {
		fl := fs.Lookup(f.Name)
		if fl == nil || !fl.Changed {
			continue
		}
		out[f.Name] = fl.Value.String()
	}
	return out
}
```

```go
// test/e2e-framework/scenario/flags.go (continued — small helper to avoid importing fmt twice)
import "fmt"

func fmtSscan(s string, p *int) (int, error) { return fmt.Sscan(s, p) }
```

> Note for the implementer: put the single `import "fmt"` with the other imports; the helper exists only to keep `RegisterFlags` readable. Merge the import blocks into one.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestRegisterAndCollectFlags -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/flags.go test/e2e-framework/scenario/flags_test.go
git commit --no-gpg-sign -m "feat(scenario): bind schema fields to pflag flags"
```

---

## Task 4: Scenario type, Runnable registry, and describe

**Files:**
- Create: `test/e2e-framework/scenario/scenario.go`
- Create: `test/e2e-framework/scenario/runnable.go`
- Create: `test/e2e-framework/scenario/registry.go`
- Create: `test/e2e-framework/scenario/describe.go`
- Test: `test/e2e-framework/scenario/runnable_test.go`

**Interfaces:**
- Consumes: `Schema`, `BuildSchema`, `Decode` (Tasks 1-2); `testing/standalone`, `testing/provisioners`, `testing/utils/common`.
- Produces:
  - `type Action[Env any] struct { Description string; NewParams func() any; Run func(ctx context.Context, env *Env, params any) error }`
  - `type Scenario[Env any] struct { Name, Description string; NewParams func() any; Provisioner func(params any) (provisioners.TypedProvisioner[Env], error); Actions map[string]Action[Env] }`
  - `type Runnable interface { Name() string; Description() string; ParamsSchema() (Schema, error); ActionSchemas() (map[string]Schema, error); Create(ctx common.Context, stack string, cfg map[string]string) error; RunAction(ctx common.Context, stack, action string, cfg map[string]string) error; Destroy(ctx common.Context, stack string) error }`
  - `func Register[Env any](s Scenario[Env])` — wraps `s` in a `genericRunnable[Env]` and stores it.
  - `func Lookup(name string) (Runnable, bool)`, `func List() []Runnable`, `func resetRegistry()` (test helper, unexported).
  - `type Description struct { ProtocolVersion int; Scenarios []ScenarioDescription }`, `type ScenarioDescription struct { Name, Description string; Params Schema; Actions map[string]Schema }`, `func Describe() (Description, error)`.

- [ ] **Step 1: Write the failing test** (uses an in-memory fake provisioner + env — no real Pulumi)

```go
// test/e2e-framework/scenario/runnable_test.go
package scenario

import (
	"context"
	"io"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// fakeEnv is a stand-in environment; actions record that they ran.
type fakeEnv struct{ ran string }

type fakeParams struct {
	Region string `scenario:"name=region,default=us-east-1,enum=us-east-1|eu-west-1"`
}
type actParams struct {
	Msg string `scenario:"name=msg,required"`
}

// fakeProvisioner satisfies provisioners.TypedProvisioner[fakeEnv] without cloud calls.
type fakeProvisioner struct{}

func (fakeProvisioner) ID() string { return "fake" }
func (fakeProvisioner) Destroy(context.Context, string, io.Writer) error { return nil }
func (fakeProvisioner) ProvisionEnv(_ context.Context, _ string, _ io.Writer, e *fakeEnv) (provisioners.RawResources, error) {
	return provisioners.RawResources{}, nil
}

func newFakeScenario() Scenario[fakeEnv] {
	return Scenario[fakeEnv]{
		Name:        "fake",
		Description: "fake scenario",
		NewParams:   func() any { return &fakeParams{} },
		Provisioner: func(any) (provisioners.TypedProvisioner[fakeEnv], error) {
			return fakeProvisioner{}, nil
		},
		Actions: map[string]Action[fakeEnv]{
			"ping": {
				Description: "ping",
				NewParams:   func() any { return &actParams{} },
				Run: func(_ context.Context, e *fakeEnv, p any) error {
					e.ran = p.(*actParams).Msg
					return nil
				},
			},
		},
	}
}

func TestRegisterDescribeAndDrive(t *testing.T) {
	resetRegistry()
	Register(newFakeScenario())

	// Describe
	d, err := Describe()
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if d.ProtocolVersion != 1 || len(d.Scenarios) != 1 {
		t.Fatalf("describe wrong: %+v", d)
	}
	if d.Scenarios[0].Params.Fields[0].Name != "region" {
		t.Fatalf("params schema wrong: %+v", d.Scenarios[0].Params)
	}
	if _, ok := d.Scenarios[0].Actions["ping"]; !ok {
		t.Fatalf("action schema missing")
	}

	// Drive create + action (fake provisioner, standalone with a temp dir)
	r, ok := Lookup("fake")
	if !ok {
		t.Fatal("scenario not found")
	}
	ctx := common.Context(newTestContext(t))
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "eu-west-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.RunAction(ctx, "fake-stack", "ping", map[string]string{"msg": "hi"}); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	// bad config rejected before provisioning
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "mars"}); err == nil {
		t.Fatal("expected enum validation error")
	}
}
```

```go
// test/e2e-framework/scenario/runnable_test.go (helper at bottom of same file)
import (
	"log"
	"os"
	"testing"
)

type testCtx struct{ dir string }

func newTestContext(t *testing.T) *testCtx { return &testCtx{dir: t.TempDir()} }
func (c *testCtx) T() *testing.T                       { return nil }
func (c *testCtx) Logf(f string, a ...any)             { log.Printf(f, a...) }
func (c *testCtx) FailNow(f string, a ...any)          { log.Fatalf(f, a...) }
func (c *testCtx) SessionOutputDir() string            { return c.dir }
var _ = os.Stderr
```

> Implementer note: `common.Context` is the interface in `testing/utils/common`. The fake above implements its methods (`T()`, `Logf`, `FailNow`, `SessionOutputDir`). If the interface has more methods, mirror `standalone.Context` in `testing/standalone/standalone.go:32-63`. Merge duplicate `testing` imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestRegisterDescribeAndDrive -v`
Expected: FAIL — `undefined: Register` / `Describe` / `Lookup` / `resetRegistry`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/scenario.go
// <license header>

package scenario

import (
	"context"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// Action is a named post-deploy operation on a running scenario. Run receives the
// fully-hydrated typed environment (same clients a test gets from s.Env()).
type Action[Env any] struct {
	Description string
	NewParams   func() any // returns a pointer to this action's tagged params struct
	Run         func(ctx context.Context, env *Env, params any) error
}

// Scenario is the single, authoritative definition of a scenario.
type Scenario[Env any] struct {
	Name        string
	Description string
	// NewParams returns a pointer to the canonical tagged params struct.
	NewParams func() any
	// Provisioner builds a provisioner from decoded params (adapter to existing
	// framework provisioners, e.g. awshost.Provisioner).
	Provisioner func(params any) (provisioners.TypedProvisioner[Env], error)
	Actions     map[string]Action[Env]
}
```

```go
// test/e2e-framework/scenario/runnable.go
// <license header>

package scenario

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Runnable is the type-erased interface the CLI and service drive.
type Runnable interface {
	Name() string
	Description() string
	ParamsSchema() (Schema, error)
	ActionSchemas() (map[string]Schema, error)
	Create(ctx common.Context, stack string, cfg map[string]string) error
	RunAction(ctx common.Context, stack, action string, cfg map[string]string) error
	Destroy(ctx common.Context, stack string) error
}

type genericRunnable[Env any] struct{ s Scenario[Env] }

func (g genericRunnable[Env]) Name() string        { return g.s.Name }
func (g genericRunnable[Env]) Description() string  { return g.s.Description }

func (g genericRunnable[Env]) ParamsSchema() (Schema, error) {
	return BuildSchema(g.s.NewParams())
}

func (g genericRunnable[Env]) ActionSchemas() (map[string]Schema, error) {
	out := map[string]Schema{}
	for name, a := range g.s.Actions {
		if a.NewParams == nil {
			out[name] = Schema{}
			continue
		}
		sc, err := BuildSchema(a.NewParams())
		if err != nil {
			return nil, fmt.Errorf("action %q: %w", name, err)
		}
		out[name] = sc
	}
	return out, nil
}

func (g genericRunnable[Env]) decodeParams(cfg map[string]string) (any, error) {
	p := g.s.NewParams()
	sc, err := BuildSchema(p)
	if err != nil {
		return nil, err
	}
	if err := Decode(sc, cfg, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (g genericRunnable[Env]) provisioner(cfg map[string]string) (any, error) {
	p, err := g.decodeParams(cfg)
	if err != nil {
		return nil, err
	}
	return g.s.Provisioner(p)
}

func (g genericRunnable[Env]) Create(ctx common.Context, stack string, cfg map[string]string) error {
	prov, err := g.provisioner(cfg)
	if err != nil {
		return err
	}
	_, err = standalone.Provision[Env](ctx, stack, prov.(interface {
		ID() string
	}).(provisionerAlias))
	return err
}
```

> Implementer note on the `Create`/`RunAction` provisioner type: `g.s.Provisioner` returns `provisioners.TypedProvisioner[Env]`, which already satisfies `provisioners.Provisioner` (the argument `standalone.Provision` wants). Do **not** use the `provisionerAlias` cast sketch above — it was shorthand. Implement cleanly as:

```go
func (g genericRunnable[Env]) buildProvisioner(cfg map[string]string) (provisioners.TypedProvisioner[Env], error) {
	p, err := g.decodeParams(cfg)
	if err != nil {
		return nil, err
	}
	return g.s.Provisioner(p)
}

func (g genericRunnable[Env]) Create(ctx common.Context, stack string, cfg map[string]string) error {
	prov, err := g.buildProvisioner(cfg)
	if err != nil {
		return err
	}
	_, err = standalone.Provision[Env](ctx, stack, prov)
	return err
}

func (g genericRunnable[Env]) RunAction(ctx common.Context, stack, action string, cfg map[string]string) error {
	a, ok := g.s.Actions[action]
	if !ok {
		return fmt.Errorf("unknown action %q for scenario %q", action, g.s.Name)
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
	// Hydrate the typed env from the running stack (idempotent up), then run.
	prov, err := g.buildProvisioner(nil) // action hydration uses scenario defaults for provisioning shape
	if err != nil {
		return err
	}
	env, err := standalone.Provision[Env](ctx, stack, prov)
	if err != nil {
		return fmt.Errorf("hydrate env for action %q: %w", action, err)
	}
	return a.Run(context.Background(), env, ap)
}

func (g genericRunnable[Env]) Destroy(ctx common.Context, stack string) error {
	prov, err := g.buildProvisioner(nil)
	if err != nil {
		return err
	}
	return standalone.Destroy(ctx, stack, prov)
}
```

> Replace the broken `Create` sketch (with `provisionerAlias`) with this clean version, and add `import "context"` to `runnable.go`. Delete the `provisionerAlias` reference entirely — it does not exist.

```go
// test/e2e-framework/scenario/registry.go
// <license header>

package scenario

import "sort"

var registry = map[string]Runnable{}

// Register adds a scenario to the package registry. Call from an init() or an
// explicit registration function in the scenario's package.
func Register[Env any](s Scenario[Env]) {
	registry[s.Name] = genericRunnable[Env]{s: s}
}

// Lookup returns the scenario registered under name.
func Lookup(name string) (Runnable, bool) {
	r, ok := registry[name]
	return r, ok
}

// List returns all registered scenarios, sorted by name.
func List() []Runnable {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Runnable, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n])
	}
	return out
}

func resetRegistry() { registry = map[string]Runnable{} }
```

```go
// test/e2e-framework/scenario/describe.go
// <license header>

package scenario

// ProtocolVersion is the version of the describe/create/action/destroy contract
// between the (version-stable) service and per-commit binaries. Bump on breaking
// changes only.
const ProtocolVersion = 1

// ScenarioDescription is the JSON-serializable description of one scenario.
type ScenarioDescription struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      Schema            `json:"params"`
	Actions     map[string]Schema `json:"actions"`
}

// Description is the full machine-readable registry description.
type Description struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	Scenarios       []ScenarioDescription `json:"scenarios"`
}

// Describe builds the registry description for `scenariorun describe --json`.
func Describe() (Description, error) {
	d := Description{ProtocolVersion: ProtocolVersion}
	for _, r := range List() {
		ps, err := r.ParamsSchema()
		if err != nil {
			return Description{}, err
		}
		as, err := r.ActionSchemas()
		if err != nil {
			return Description{}, err
		}
		d.Scenarios = append(d.Scenarios, ScenarioDescription{
			Name:        r.Name(),
			Description: r.Description(),
			Params:      ps,
			Actions:     as,
		})
	}
	return d, nil
}
```

> Implementer note: add `json` struct tags to `Field`/`Schema`/`Kind` in `schema.go` so `describe --json` is clean (`Name string \`json:"name"\``, etc.). This is a mechanical edit to Task 1's types; do it here.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/ -run TestRegisterDescribeAndDrive -v`
Expected: PASS. (If `standalone.Provision` with the fake provisioner attempts no cloud work, it returns the built env; the fake `ProvisionEnv` returns empty resources so `BuildEnvFromResources` is a no-op.)

> If `standalone.Provision` requires more of `common.Context` than the fake implements, extend the fake to match `standalone.Context`'s method set (see `standalone.go:32-63`).

- [ ] **Step 5: Run the full package and commit**

Run: `cd test/e2e-framework && go test ./scenario/ -v`
Expected: PASS (all of Tasks 1-4).

```bash
git add test/e2e-framework/scenario/
git commit --no-gpg-sign -m "feat(scenario): generic Scenario type, Runnable registry, describe"
```

---

## Task 5: Reusable component — AgentParams

**Files:**
- Create: `test/e2e-framework/scenario/params/agent.go`
- Test: `test/e2e-framework/scenario/params/agent_test.go`

**Interfaces:**
- Consumes: `components/datadog/agentparams` (`agentparams.Option`, `WithVersion`, `WithFlavor`, `WithAgentConfig`, `WithPipeline`, `WithFakeintake`).
- Produces:
  - `type AgentParams struct { Version, Flavor, ConfigPath, PipelineID string; Fakeintake, Install bool; AdvancedOptions []agentparams.Option }` with `scenario` tags matching `create-vm` flag names (`agent-version`, `agent-flavor`, `agent-config-path`, `pipeline-id`, `use-fakeintake`, `install-agent`); `AdvancedOptions` tagged `scenario:"-"`.
  - `func (a AgentParams) ToOptions() ([]agentparams.Option, error)` — maps set fields to `agentparams.WithX`, reads `ConfigPath` file content for `WithAgentConfig`, appends `AdvancedOptions`. Returns `nil` options (not an error) when `Install` is false — caller decides whether to install.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/params/agent_test.go
package params

import "testing"

func TestAgentParamsToOptions(t *testing.T) {
	a := AgentParams{Version: "7.42.0", Flavor: "datadog-fips-agent", Install: true}
	opts, err := a.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	// version + flavor => 2 options
	if len(opts) != 2 {
		t.Fatalf("want 2 options, got %d", len(opts))
	}
}

func TestAgentParamsConfigPathMissing(t *testing.T) {
	a := AgentParams{ConfigPath: "/does/not/exist.yaml", Install: true}
	if _, err := a.ToOptions(); err == nil {
		t.Fatal("expected error reading missing config path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/params/ -run TestAgentParams -v`
Expected: FAIL — `undefined: AgentParams`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/params/agent.go
// <license header>

// Package params holds reusable, tagged scenario parameter components (agent,
// fakeintake, …) that embed into a scenario's canonical params struct.
package params

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
)

// AgentParams is the reusable agent-configuration component. Its tagged fields
// cover at least the create-vm agent surface and convert to agentparams.Option.
type AgentParams struct {
	Version    string `scenario:"name=agent-version,help=Agent version (e.g. 7.42.0~rc.1-1); empty installs latest"`
	Flavor     string `scenario:"name=agent-flavor,help=Agent package flavor,enum=datadog-agent|datadog-fips-agent"`
	ConfigPath string `scenario:"name=agent-config-path,help=Path to a datadog.yaml whose contents are applied"`
	PipelineID string `scenario:"name=pipeline-id,help=GitLab pipeline build to install"`
	Fakeintake bool   `scenario:"name=use-fakeintake,help=Point the agent at a provisioned fakeintake"`
	Install    bool   `scenario:"name=install-agent,default=true,help=Install the agent"`

	// AdvancedOptions is a Go-only escape hatch for the full agentparams surface.
	AdvancedOptions []agentparams.Option `scenario:"-"`
}

// ToOptions converts the component to agentparams options.
func (a AgentParams) ToOptions() ([]agentparams.Option, error) {
	var opts []agentparams.Option
	if a.Version != "" {
		opts = append(opts, agentparams.WithVersion(a.Version))
	}
	if a.Flavor != "" {
		opts = append(opts, agentparams.WithFlavor(a.Flavor))
	}
	if a.PipelineID != "" {
		opts = append(opts, agentparams.WithPipeline(a.PipelineID))
	}
	if a.ConfigPath != "" {
		content, err := os.ReadFile(a.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("reading agent-config-path: %w", err)
		}
		opts = append(opts, agentparams.WithAgentConfig(string(content)))
	}
	opts = append(opts, a.AdvancedOptions...)
	return opts, nil
}
```

> Implementer note: confirm exact `agentparams` option signatures against `components/datadog/agentparams/params.go` (e.g. `WithFlavor(string)`, `WithPipeline(string)`). Adjust if a signature differs. `Fakeintake` is wired at the scenario level (the EC2 scenario decides fakeintake via its own provisioner option), so it is intentionally not mapped here.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/params/ -run TestAgentParams -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/params/agent.go test/e2e-framework/scenario/params/agent_test.go
git commit --no-gpg-sign -m "feat(scenario): reusable AgentParams component"
```

---

## Task 6: Reusable component — FakeintakeParams

**Files:**
- Create: `test/e2e-framework/scenario/params/fakeintake.go`
- Test: `test/e2e-framework/scenario/params/fakeintake_test.go`

**Interfaces:**
- Consumes: `components/datadog/fakeintake` (`fakeintake.Option`); confirm package path/option names in `scenarios/aws/fakeintake` / `components/datadog/fakeintake`.
- Produces:
  - `type FakeintakeParams struct { Enabled bool; AdvancedOptions []fakeintake.Option }` with `scenario` tags (`use-fakeintake`); `AdvancedOptions` tagged `scenario:"-"`.
  - `func (f FakeintakeParams) ToOptions() ([]fakeintake.Option, error)`.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/params/fakeintake_test.go
package params

import "testing"

func TestFakeintakeParamsToOptions(t *testing.T) {
	f := FakeintakeParams{Enabled: true}
	opts, err := f.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	if opts == nil {
		// nil is acceptable (no extra options); just assert no panic/error path.
		_ = opts
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/params/ -run TestFakeintakeParams -v`
Expected: FAIL — `undefined: FakeintakeParams`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/params/fakeintake.go
// <license header>

package params

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
)

// FakeintakeParams is the reusable fakeintake component.
type FakeintakeParams struct {
	Enabled bool `scenario:"name=use-fakeintake,default=false,help=Provision a fakeintake and point the agent at it"`

	// AdvancedOptions is a Go-only escape hatch for the full fakeintake surface.
	AdvancedOptions []fakeintake.Option `scenario:"-"`
}

// ToOptions converts the component to fakeintake options.
func (f FakeintakeParams) ToOptions() ([]fakeintake.Option, error) {
	return f.AdvancedOptions, nil
}
```

> Implementer note: verify the fakeintake component import path and `Option` type. In the ec2 scenario, fakeintake options pass through `ec2.WithFakeIntakeOptions(...)`; if `fakeintake.Option` is not the right type there, align `FakeintakeParams.AdvancedOptions` to whatever `ec2.WithFakeIntakeOptions` accepts (see `scenarios/aws/ec2/run_args.go`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./scenario/params/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/params/fakeintake.go test/e2e-framework/scenario/params/fakeintake_test.go
git commit --no-gpg-sign -m "feat(scenario): reusable FakeintakeParams component"
```

---

## Task 7: Reference scenario — AWS EC2 host

**Files:**
- Create: `test/e2e-framework/scenario/scenarios/ec2host/params.go`
- Create: `test/e2e-framework/scenario/scenarios/ec2host/scenario.go`
- Test: `test/e2e-framework/scenario/scenarios/ec2host/scenario_test.go`

**Interfaces:**
- Consumes: `scenario` (Task 4), `scenario/params` (Tasks 5-6), `scenarios/aws/ec2` (`ec2.Option`, `ec2.WithOS`, `ec2.WithAgentOptions`, `ec2.WithFakeIntakeOptions`, `ec2.WithoutFakeIntake`), `testing/provisioners/aws/host` (`awshost.Provisioner`, `awshost.WithRunOptions`), `testing/environments` (`environments.Host`), `components/os` (`e2eos`), `components/datadog/agentparams`.
- Produces:
  - `type EC2HostParams struct { OS, Arch string; Agent params.AgentParams; Fakeintake params.FakeintakeParams; InstanceOptions []ec2.VMOption }` (InstanceOptions tagged `scenario:"-"`).
  - `func Provisioner(p *EC2HostParams) (provisioners.TypedProvisioner[environments.Host], error)`.
  - `func Scenario() scenario.Scenario[environments.Host]` and `func Register()` (calls `scenario.Register(Scenario())`).
  - Actions: `restart-agent` (no params), `run-command` (`RunCommandParams{ Command string }`).

> **Two-path note:** existing tests keep using `awshost.Provisioner(...)` with typed `With…` options unchanged — they do not use `EC2HostParams`. `Provisioner(p *EC2HostParams)` is the CLI/service adapter that maps the tagged struct onto the same `ec2`/`agentparams` typed options and the same `awshost.Provisioner`.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e-framework/scenario/scenarios/ec2host/scenario_test.go
package ec2host

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

func TestScenarioSchemaExposesAgentFlags(t *testing.T) {
	s := Scenario()
	sc, err := scenario.BuildSchema(s.NewParams())
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	names := map[string]bool{}
	for _, f := range sc.Fields {
		names[f.Name] = true
	}
	for _, want := range []string{"os", "arch", "agent-version", "agent-flavor", "agent-config-path", "use-fakeintake"} {
		if !names[want] {
			t.Errorf("expected flag %q in schema, missing (got %v)", want, names)
		}
	}
}

func TestProvisionerBuilds(t *testing.T) {
	p := &EC2HostParams{OS: "ubuntu-22.04", Arch: "x86_64"}
	prov, err := Provisioner(p)
	if err != nil {
		t.Fatalf("Provisioner: %v", err)
	}
	if prov == nil {
		t.Fatal("nil provisioner")
	}
}

func TestActionsRegistered(t *testing.T) {
	s := Scenario()
	if _, ok := s.Actions["restart-agent"]; !ok {
		t.Error("restart-agent action missing")
	}
	if _, ok := s.Actions["run-command"]; !ok {
		t.Error("run-command action missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./scenario/scenarios/ec2host/ -v`
Expected: FAIL — `undefined: Scenario` / `EC2HostParams` / `Provisioner`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/scenario/scenarios/ec2host/params.go
// <license header>

// Package ec2host is the reference unified scenario: an AWS EC2 host with the agent.
package ec2host

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

// EC2HostParams is the canonical, introspectable parameter set for the ec2-host scenario.
type EC2HostParams struct {
	OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
	Arch string `scenario:"name=arch,default=x86_64,help=CPU architecture,enum=x86_64|arm64"`

	Agent      params.AgentParams      // embedded → agent flags
	Fakeintake params.FakeintakeParams // embedded → fakeintake flags

	// Go-only escape hatch for advanced VM tuning.
	InstanceOptions []ec2.VMOption `scenario:"-"`
}
```

```go
// test/e2e-framework/scenario/scenarios/ec2host/scenario.go
// <license header>

package ec2host

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// osDescriptor maps the schema enum value to the framework OS descriptor.
func osDescriptor(name string) (oscomp.Descriptor, error) {
	switch name {
	case "ubuntu-22.04":
		return oscomp.Ubuntu2204, nil
	case "debian-12":
		return oscomp.Debian12, nil
	case "amazon-linux-2023":
		return oscomp.AmazonLinux2023, nil
	default:
		return oscomp.Descriptor{}, fmt.Errorf("unknown os %q", name)
	}
}

// Provisioner adapts canonical params to the existing awshost provisioner.
func Provisioner(p *EC2HostParams) (provisioners.TypedProvisioner[environments.Host], error) {
	osDesc, err := osDescriptor(p.OS)
	if err != nil {
		return nil, err
	}
	agentOpts, err := p.Agent.ToOptions()
	if err != nil {
		return nil, err
	}
	fakeOpts, err := p.Fakeintake.ToOptions()
	if err != nil {
		return nil, err
	}

	runOpts := []ec2.Option{
		ec2.WithEC2InstanceOptions(append([]ec2.VMOption{ec2.WithOS(osDesc)}, p.InstanceOptions...)...),
	}
	if p.Agent.Install {
		runOpts = append(runOpts, ec2.WithAgentOptions(agentOpts...))
	}
	if p.Agent.Fakeintake || p.Fakeintake.Enabled {
		runOpts = append(runOpts, ec2.WithFakeIntakeOptions(fakeOpts...))
	} else {
		runOpts = append(runOpts, ec2.WithoutFakeIntake())
	}

	return awshost.Provisioner(awshost.WithRunOptions(runOpts...)), nil
}

// RunCommandParams are the parameters for the run-command action.
type RunCommandParams struct {
	Command string `scenario:"name=command,required,help=Shell command to run over SSH"`
}

// Scenario returns the unified ec2-host scenario definition.
func Scenario() scenario.Scenario[environments.Host] {
	return scenario.Scenario[environments.Host]{
		Name:        "ec2-host",
		Description: "AWS EC2 VM with the Datadog Agent",
		NewParams:   func() any { return &EC2HostParams{} },
		Provisioner: func(p any) (provisioners.TypedProvisioner[environments.Host], error) {
			return Provisioner(p.(*EC2HostParams))
		},
		Actions: map[string]scenario.Action[environments.Host]{
			"restart-agent": {
				Description: "Restart the Datadog Agent",
				Run: func(ctx context.Context, env *environments.Host, _ any) error {
					return env.Agent.Client().RestartAgent(ctx)
				},
			},
			"run-command": {
				Description: "Run a shell command on the host over SSH",
				NewParams:   func() any { return &RunCommandParams{} },
				Run: func(_ context.Context, env *environments.Host, p any) error {
					_, err := env.RemoteHost.Execute(p.(*RunCommandParams).Command)
					return err
				},
			},
		},
	}
}

// Register registers the ec2-host scenario in the package registry.
func Register() { scenario.Register(Scenario()) }
```

> Implementer notes (verify against the codebase, adjust names as needed):
> - OS descriptor symbols live in `components/os` — confirm `Ubuntu2204`, `Debian12`, `AmazonLinux2023` and the descriptor type name; the import alias is `e2eos` in some files.
> - `env.Agent.Client().RestartAgent(ctx)` — confirm the agent client method (may be `RestartAgent()` without ctx, or via `env.Agent.Client.RestartAgent`). Match `environments.Host`'s `Agent` field API.
> - `env.RemoteHost.Execute(cmd)` returns `(string, error)` per the standalone docs.
> - `ec2.WithEC2InstanceOptions`, `ec2.WithOS`, `ec2.WithAgentOptions`, `ec2.WithFakeIntakeOptions`, `ec2.WithoutFakeIntake` are in `scenarios/aws/ec2/run_args.go` / `vm.go` — confirm exact names.

- [ ] **Step 4: Run tests**

Run: `cd test/e2e-framework && go test ./scenario/scenarios/ec2host/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/scenario/scenarios/ec2host/
git commit --no-gpg-sign -m "feat(scenario): ec2-host reference scenario with actions"
```

---

## Task 8: E2E test driving the reference scenario

**Files:**
- Create: `test/new-e2e/tests/scenario-model/ec2host_test.go`

**Interfaces:**
- Consumes: `ec2host.Provisioner`, `ec2host.Scenario`, `ec2host.EC2HostParams`, `ec2host.RunCommandParams` (Task 7); `e2e.BaseSuite`, `e2e.Run`, `e2e.WithProvisioner`; `environments.Host`.
- Produces: an E2E suite proving (a) the scenario's provisioner works through `e2e.Run`, and (b) an action handler runs against the live `s.Env()`.

> This test provisions real cloud infra; it runs only under the E2E gating (`dda inv new-e2e-tests.run`), not in unit-test CI.

- [ ] **Step 1: Write the test**

```go
// test/new-e2e/tests/scenario-model/ec2host_test.go
// <license header>

package scenariomodel

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/require"
)

type ec2HostSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestEC2HostScenario(t *testing.T) {
	t.Parallel()
	prov, err := ec2host.Provisioner(&ec2host.EC2HostParams{OS: "ubuntu-22.04", Arch: "x86_64"})
	require.NoError(t, err)
	e2e.Run(t, &ec2HostSuite{}, e2e.WithProvisioner(prov))
}

func (s *ec2HostSuite) TestActionRunsAgainstEnv() {
	// The same action handler the CLI/service invoke, called against the live env.
	runCommand := ec2host.Scenario().Actions["run-command"]
	err := runCommand.Run(context.Background(), s.Env(), &ec2host.RunCommandParams{Command: "echo hello"})
	s.Require().NoError(err)
}
```

- [ ] **Step 2: Compile-check the test (no provisioning)**

Run: `cd test/new-e2e && go vet ./tests/scenario-model/...`
Expected: no errors. (Compilation proves the cross-module wiring; do not run the suite in this step.)

- [ ] **Step 3: (Optional, gated) run the real E2E suite**

Run: `dda inv new-e2e-tests.run --targets=./tests/scenario-model/...`
Expected: provisions a VM, agent installs, action runs, PASS. (~10 min, requires AWS creds.)

- [ ] **Step 4: Commit**

```bash
git add test/new-e2e/tests/scenario-model/ec2host_test.go
git commit --no-gpg-sign -m "test(scenario): e2e suite driving ec2-host scenario + action"
```

---

## Task 9: CLI binary (`scenariorun`) + `dda lab` forwarder

**Files:**
- Create: `test/e2e-framework/cmd/scenariorun/main.go`
- Create: `test/e2e-framework/cmd/scenariorun/import_scenarios.go`
- Create: `test/e2e-framework/cmd/scenariorun/main_test.go`
- Create: `tasks/scenario.py`
- Modify: `tasks/__init__.py` (register the collection)

**Interfaces:**
- Consumes: `scenario` (Tasks 1-4), `ec2host.Register` (Task 7), `spf13/cobra`, `testing/standalone` (`NewContext`).
- Produces:
  - `func rootCmd() *cobra.Command` building `list`, `describe`, `create <scenario>`, `action <scenario> <action>`, `destroy <scenario>`, with per-scenario flags registered from the schema.
  - `dda lab` invoke task that builds `bin/scenariorun` (plain `go build` inside the module, mirroring `tasks/ai_sandbox.py:120-126`) and forwards argv to it.

- [ ] **Step 1: Write the failing test** (verifies dynamic command tree + flag registration, no provisioning)

```go
// test/e2e-framework/cmd/scenariorun/main_test.go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
)

func TestDescribeCommandListsScenario(t *testing.T) {
	ec2host.Register()
	root := rootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"describe", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute describe: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "\"ec2-host\"") || !strings.Contains(s, "\"protocolVersion\": 1") {
		t.Fatalf("describe output missing scenario/protocol: %s", s)
	}
}

func TestCreateCommandHasSchemaFlags(t *testing.T) {
	ec2host.Register()
	root := rootCmd()
	// find: create ec2-host --help should list --os, --agent-version
	create, _, err := root.Find([]string{"create", "ec2-host"})
	if err != nil {
		t.Fatalf("find create ec2-host: %v", err)
	}
	for _, want := range []string{"os", "agent-version", "use-fakeintake"} {
		if create.Flags().Lookup(want) == nil {
			t.Errorf("create ec2-host missing --%s flag", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./cmd/scenariorun/ -v`
Expected: FAIL — `undefined: rootCmd`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/cmd/scenariorun/import_scenarios.go
// <license header>

package main

import "github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"

// registerScenarios registers all built-in scenarios. Add new scenarios here.
func registerScenarios() {
	ec2host.Register()
}
```

```go
// test/e2e-framework/cmd/scenariorun/main.go
// <license header>

// Command scenariorun is the dynamic CLI over the scenario registry. Its command
// tree (flags included) is built by reflecting each scenario's tagged params.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/spf13/cobra"
)

func main() {
	registerScenarios()
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "scenariorun", Short: "Drive e2e scenarios"}
	root.AddCommand(listCmd(), describeCmd(), createCmd(), actionCmd(), destroyCmd())
	return root
}

func newCtx() *standalone.Context {
	dir, _ := os.MkdirTemp("", "scenariorun-")
	return standalone.NewContext(dir)
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scenarios",
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, r := range scenario.List() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", r.Name(), r.Description())
			}
			return nil
		},
	}
}

func describeCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "describe",
		Short: "Describe scenarios (machine-readable with --json)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := scenario.Describe()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(d)
			}
			for _, sd := range d.Scenarios {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", sd.Name, sd.Description)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON schema")
	return c
}

// createCmd builds one subcommand per scenario, each with schema-derived flags.
func createCmd() *cobra.Command {
	c := &cobra.Command{Use: "create <scenario>", Short: "Provision a scenario"}
	for _, r := range scenario.List() {
		r := r
		sc, err := r.ParamsSchema()
		if err != nil {
			panic(err)
		}
		sub := &cobra.Command{
			Use:   r.Name(),
			Short: r.Description(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg := scenario.CollectFlags(sc, cmd.Flags())
				stack, _ := cmd.Flags().GetString("stack")
				return r.Create(newCtx(), stack, cfg)
			},
		}
		scenario.RegisterFlags(sc, sub.Flags())
		sub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
		c.AddCommand(sub)
	}
	return c
}

func actionCmd() *cobra.Command {
	c := &cobra.Command{Use: "action <scenario> <action>", Short: "Run a scenario action"}
	for _, r := range scenario.List() {
		r := r
		actions, err := r.ActionSchemas()
		if err != nil {
			panic(err)
		}
		scenarioCmd := &cobra.Command{Use: r.Name(), Short: r.Description()}
		for name, asc := range actions {
			name, asc := name, asc
			actSub := &cobra.Command{
				Use: name,
				RunE: func(cmd *cobra.Command, _ []string) error {
					cfg := scenario.CollectFlags(asc, cmd.Flags())
					stack, _ := cmd.Flags().GetString("stack")
					return r.RunAction(newCtx(), stack, name, cfg)
				},
			}
			scenario.RegisterFlags(asc, actSub.Flags())
			actSub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
			scenarioCmd.AddCommand(actSub)
		}
		c.AddCommand(scenarioCmd)
	}
	return c
}

func destroyCmd() *cobra.Command {
	c := &cobra.Command{Use: "destroy <scenario>", Short: "Tear down a scenario"}
	for _, r := range scenario.List() {
		r := r
		sub := &cobra.Command{
			Use: r.Name(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				stack, _ := cmd.Flags().GetString("stack")
				return r.Destroy(newCtx(), stack)
			},
		}
		sub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
		c.AddCommand(sub)
	}
	return c
}
```

> Implementer note: command builders read the registry at construction time, so `registerScenarios()` (or `ec2host.Register()` in tests) must run **before** `rootCmd()`. The tests call `ec2host.Register()` first; `main()` calls `registerScenarios()` first.

```python
# tasks/scenario.py
"""
`dda lab` forwarder: builds the scenariorun binary in the test/e2e-framework Go
module and forwards arguments to it. Mirrors tasks/ai_sandbox.py.
"""

import os

from invoke import Collection, task

E2E_FRAMEWORK_DIR = "test/e2e-framework"
CLI_PACKAGE = "./cmd/scenariorun"
CLI_BIN = "bin/scenariorun"


@task(
    auto_shortflags=False,
    help={"args": "Arguments forwarded verbatim to the scenariorun binary"},
)
def lab(ctx, args=""):
    """Build (if needed) and run the scenariorun CLI, forwarding ARGS.

    Example: dda inv lab --args="create ec2-host --os debian-12"
    """
    os.makedirs(os.path.join(E2E_FRAMEWORK_DIR, os.path.dirname(CLI_BIN)), exist_ok=True)
    with ctx.cd(E2E_FRAMEWORK_DIR):
        ctx.run(f"go build -o {CLI_BIN} {CLI_PACKAGE}")
        ctx.run(f"./{CLI_BIN} {args}", pty=True)


collection = Collection("lab")
collection.add_task(lab)
```

```python
# tasks/__init__.py  — additions
# (near the other e2e-framework collection imports, ~line 103)
from tasks import scenario as scenario_tasks

# (near the other add_collection calls, ~line 284)
ns.add_collection(scenario_tasks.collection, "lab")
```

> Implementer note: confirm the exact registration idiom used by neighboring collections in `tasks/__init__.py` and match it (some are added via `ns.add_collection(x.collection, "name")`). The forwarder uses `--args="…"`; a future refinement can use `invoke`'s raw-argv passthrough for a nicer `dda lab create …` UX.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./cmd/scenariorun/ -v`
Expected: PASS.

- [ ] **Step 5: Smoke-build the binary and the forwarder**

Run:
```bash
cd test/e2e-framework && go build -o bin/scenariorun ./cmd/scenariorun && ./bin/scenariorun describe --json | head -20
```
Expected: prints JSON containing `"ec2-host"` and `"protocolVersion": 1`.

- [ ] **Step 6: Commit**

```bash
git add test/e2e-framework/cmd/scenariorun/ tasks/scenario.py tasks/__init__.py
git commit --no-gpg-sign -m "feat(scenario): dynamic scenariorun CLI + dda lab forwarder"
```

---

## Task 10: Service stub (per-commit build + drive)

**Files:**
- Create: `test/e2e-framework/cmd/scenario-service/main.go`
- Create: `test/e2e-framework/cmd/scenario-service/builder.go`
- Create: `test/e2e-framework/cmd/scenario-service/builder_test.go`

**Interfaces:**
- Consumes: stdlib `net/http`, `os/exec`, `encoding/json`.
- Produces:
  - `type Builder interface { Build(commit string) (binPath string, err error) }` and a `gitBuilder` implementation (worktree checkout + `go build` of `./cmd/scenariorun`, cached by commit).
  - `type Driver struct { Builder Builder }` with `Describe(commit string) ([]byte, error)`, `Run(commit, scenario string, cfg map[string]string, stack string) error`, `Action(...)`, `Destroy(...)` — all shelling to the per-commit binary.
  - HTTP handlers: `POST /runs`, `GET /runs/{id}` (status only in stub), `POST /runs/{id}/actions/{action}`, `DELETE /runs/{id}`.

- [ ] **Step 1: Write the failing test** (uses a fake Builder pointing at a stub binary — no real git/build)

```go
// test/e2e-framework/cmd/scenario-service/builder_test.go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeBuilder returns a tiny shell script that echoes its args, standing in for
// a per-commit scenariorun binary.
type fakeBuilder struct{ bin string }

func (f fakeBuilder) Build(string) (string, error) { return f.bin, nil }

func writeStubBinary(t *testing.T) string {
	dir := t.TempDir()
	bin := filepath.Join(dir, "scenariorun")
	script := "#!/bin/sh\necho \"called: $*\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDriverShellsToBinary(t *testing.T) {
	bin := writeStubBinary(t)
	d := Driver{Builder: fakeBuilder{bin: bin}}
	out, err := d.Describe("abc123")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if want := "called: describe --json"; string(out) != "called: describe --json\n" {
		t.Fatalf("unexpected describe output: %q (want contains %q)", out, want)
	}
	// sanity: the stub binary is actually executable
	if _, err := exec.LookPath(bin); err != nil && !filepath.IsAbs(bin) {
		t.Fatalf("stub not executable: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/e2e-framework && go test ./cmd/scenario-service/ -run TestDriverShellsToBinary -v`
Expected: FAIL — `undefined: Driver`.

- [ ] **Step 3: Write minimal implementation**

```go
// test/e2e-framework/cmd/scenario-service/builder.go
// <license header>

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Builder produces a scenariorun binary for a given commit.
type Builder interface {
	Build(commit string) (binPath string, err error)
}

// gitBuilder checks out a commit into a worktree and builds ./cmd/scenariorun,
// caching the resulting binary by commit. Repo root and cache dir are configurable.
type gitBuilder struct {
	repoRoot string
	cacheDir string
	mu       sync.Mutex
	cache    map[string]string
}

func newGitBuilder(repoRoot, cacheDir string) *gitBuilder {
	return &gitBuilder{repoRoot: repoRoot, cacheDir: cacheDir, cache: map[string]string{}}
}

func (b *gitBuilder) Build(commit string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if p, ok := b.cache[commit]; ok {
		return p, nil
	}
	wt := filepath.Join(b.cacheDir, "wt-"+commit)
	if err := run(b.repoRoot, "git", "worktree", "add", "--detach", wt, commit); err != nil {
		return "", fmt.Errorf("git worktree add %s: %w", commit, err)
	}
	bin := filepath.Join(b.cacheDir, "scenariorun-"+commit)
	mod := filepath.Join(wt, "test", "e2e-framework")
	if err := run(mod, "go", "build", "-o", bin, "./cmd/scenariorun"); err != nil {
		return "", fmt.Errorf("build scenariorun @ %s: %w", commit, err)
	}
	b.cache[commit] = bin
	return bin, nil
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Driver drives a per-commit scenariorun binary via its stable CLI protocol.
type Driver struct{ Builder Builder }

func (d Driver) exec(commit string, args ...string) ([]byte, error) {
	bin, err := d.Builder.Build(commit)
	if err != nil {
		return nil, err
	}
	return exec.Command(bin, args...).Output()
}

// Describe returns the per-commit binary's describe --json output.
func (d Driver) Describe(commit string) ([]byte, error) {
	return d.exec(commit, "describe", "--json")
}

// Run provisions a scenario at a commit with the given config + stack name.
func (d Driver) Run(commit, scenarioName, stack string, cfg map[string]string) error {
	args := []string{"create", scenarioName, "--stack", stack}
	for k, v := range cfg {
		args = append(args, "--"+k, v)
	}
	_, err := d.exec(commit, args...)
	return err
}

// Action runs a named action on a running stack.
func (d Driver) Action(commit, scenarioName, action, stack string, cfg map[string]string) error {
	args := []string{"action", scenarioName, action, "--stack", stack}
	for k, v := range cfg {
		args = append(args, "--"+k, v)
	}
	_, err := d.exec(commit, args...)
	return err
}

// Destroy tears down a running stack.
func (d Driver) Destroy(commit, scenarioName, stack string) error {
	_, err := d.exec(commit, "destroy", scenarioName, "--stack", stack)
	return err
}
```

```go
// test/e2e-framework/cmd/scenario-service/main.go
// <license header>

// Command scenario-service is a stub long-running service that builds and drives
// scenariorun binaries from caller-specified commits. It demonstrates the
// commit -> build -> execute loop; production concerns (auth, persistence,
// async jobs, build farm) are intentionally out of scope.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

type runRequest struct {
	Commit   string            `json:"commit"`
	Scenario string            `json:"scenario"`
	Config   map[string]string `json:"config"`
	Action   string            `json:"action,omitempty"`
}

type runRecord struct {
	ID       string `json:"run_id"`
	Stack    string `json:"stack_id"`
	Commit   string `json:"commit"`
	Scenario string `json:"scenario"`
	Status   string `json:"status"`
}

type server struct {
	driver Driver
	mu     sync.Mutex
	runs   map[string]*runRecord
	seq    int
}

func (s *server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Commit == "" || req.Scenario == "" {
		http.Error(w, "commit and scenario are required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("run-%d", s.seq)
	stack := fmt.Sprintf("%s-%s", req.Scenario, id)
	rec := &runRecord{ID: id, Stack: stack, Commit: req.Commit, Scenario: req.Scenario, Status: "provisioning"}
	s.runs[id] = rec
	s.mu.Unlock()

	if err := s.driver.Run(req.Commit, req.Scenario, stack, req.Config); err != nil {
		rec.Status = "failed"
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec.Status = "running"
	writeJSON(w, http.StatusCreated, rec)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := os.Getenv("SCENARIO_SERVICE_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	repoRoot := os.Getenv("SCENARIO_SERVICE_REPO")
	if repoRoot == "" {
		repoRoot = "."
	}
	cacheDir, _ := os.MkdirTemp("", "scenario-service-cache-")
	s := &server{driver: Driver{Builder: newGitBuilder(repoRoot, cacheDir)}, runs: map[string]*runRecord{}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	log.Printf("scenario-service listening on %s (repo=%s)", addr, repoRoot)
	log.Fatal(http.ListenAndServe(addr, mux))
}
```

> Implementer note: the stub implements `POST /runs` end-to-end (the core commit→build→execute proof). `GET /runs/{id}`, `POST /runs/{id}/actions/{action}`, and `DELETE /runs/{id}` are straightforward follow-on handlers using `s.runs[id]` + `s.driver.Action/Destroy`; add them with the same `writeJSON` helper. Keep them in `main.go`. The `Go 1.22+ method-pattern mux` (`"POST /runs"`) requires the module's Go version ≥ 1.22 — confirm in `go.mod`; if lower, route by `r.Method` inside a single handler.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd test/e2e-framework && go test ./cmd/scenario-service/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e-framework/cmd/scenario-service/
git commit --no-gpg-sign -m "feat(scenario): per-commit build-and-drive service stub"
```

---

## Task 11: Documentation

**Files:**
- Modify: `test/e2e-framework/AGENTS.md`

**Interfaces:**
- Consumes: nothing (docs).
- Produces: a "Unified scenario model" section so future agents discover the pattern.

- [ ] **Step 1: Add the documentation section**

Append to `test/e2e-framework/AGENTS.md` (before "## Keeping this file accurate"):

```markdown
## Unified scenario model

A scenario can be defined **once** in Go and driven from tests, the `scenariorun`
CLI, and the `scenario-service` stub. See `scenario/` (reflection + `Scenario[Env]`
+ registry), `scenario/params/` (reusable `AgentParams`/`FakeintakeParams`
components), and `scenario/scenarios/ec2host/` (reference scenario).

- Canonical params are a tagged struct (`scenario:"name=…,default=…,help=…,enum=a|b,required"`;
  `scenario:"-"` = Go-only escape hatch). Build the schema with `scenario.BuildSchema`.
- Reusable components (agent, fakeintake) embed into the canonical struct and expose
  `ToOptions()`.
- Actions receive the fully-hydrated typed env (same clients as `s.Env()`), hydrated
  via `testing/standalone`.
- CLI: `dda inv lab --args="create ec2-host --os debian-12"`. The CLI command tree
  and flags are generated from the registry by reflection — never hand-declared.
- Service: builds and drives the `scenariorun` binary from a caller-specified commit
  via the stable `describe`/`create`/`action`/`destroy` protocol (`describe --json`
  carries `protocolVersion`).
```

- [ ] **Step 2: Commit**

```bash
git add test/e2e-framework/AGENTS.md
git commit --no-gpg-sign -m "docs(scenario): document the unified scenario model"
```

---

## Self-Review

**1. Spec coverage:**
- Define-once / tagged-struct canonical (CLI projection) → Tasks 1, 7. ✓
- Go-native introspection → Tasks 1-4 (schema/decode/flags/describe). ✓
- Two convergent provisioning paths (tests keep existing typed options; CLI maps onto them, zero migration) → Task 7 (`Provisioner(p)` adapter over `awshost.Provisioner`), Task 8 (E2E parity). ✓
- Reusable param components (agent + fakeintake) → Tasks 5, 6, embedded in Task 7. ✓ (covers the "same for fakeintake" requirement)
- Agent config at least as good as `create-vm` → Task 5 field set mirrors `create-vm` flags. ✓
- Actions with hydrated typed-env client reuse → Tasks 4 (`RunAction` via `standalone.Provision`), 7 (`restart-agent`/`run-command`), 8 (test calls action against `s.Env()`). ✓
- Go-is-the-CLI + `dda` thin forwarder → Task 9. ✓
- E2E test parity through the existing typed provisioner → Task 8. ✓
- Long-running, version-agnostic, per-commit build+drive service via stable protocol + `protocolVersion` → Tasks 4 (`ProtocolVersion`), 10. ✓
- Reference scenario = AWS EC2 host → Task 7. ✓
- Error handling (registration-time, validation-before-provision, runtime, service build-vs-scenario) → Tasks 2 (validation), 4 (unknown action), 10 (build vs run error paths). ✓
- Testing strategy (reflection unit tests highest value; CLI table tests with fake; service handler tests with fake; reference E2E parity) → Tasks 1-4, 9, 10, 8. ✓
- Docs → Task 11. ✓

**2. Placeholder scan:** No "TBD"/"implement later". Two spots intentionally flag follow-on handlers (service `GET`/`DELETE`/action endpoints) and richer `dda lab` argv UX — both are explicitly out of the stub's critical path per the spec's "design-for, deferred" list, and the core handler is fully implemented. The Task 4 `runnable.go` first `Create` sketch is explicitly marked broken and replaced with a clean version in the same task.

**3. Type consistency:** `Schema`/`Field`/`Kind` consistent across Tasks 1-4, 9. `Runnable` methods (`Create`/`RunAction`/`Destroy`/`ParamsSchema`/`ActionSchemas`) consistent between Task 4 definition and Task 9 usage. `Scenario[Env]`/`Action[Env]` fields (`NewParams`, `Provisioner`, `Run`) consistent between Tasks 4 and 7. `AgentParams.ToOptions`/`FakeintakeParams.ToOptions` consistent between Tasks 5/6 and 7. `Driver` methods consistent between Task 10 definition and test. `Provisioner(p *EC2HostParams)` signature consistent between Tasks 7 and 8.

**Known verification points for the implementer** (flagged inline; not plan gaps): exact `agentparams`/`ec2`/`fakeintake` option names, `components/os` descriptor symbols, the `environments.Host` agent-client method name, the `common.Context` method set, the module's Go version for the `net/http` method-pattern mux, and the `tasks/__init__.py` collection-registration idiom. Each is a small, local confirmation against named files.
