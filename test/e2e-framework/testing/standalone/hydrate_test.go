// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package standalone

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// minImportable is a minimal Importable type for testing key replay.
type minImportable struct {
	components.JSONImporter
	Value string `json:"value"`
}

func (m *minImportable) Import(in []byte, obj any) error {
	return json.Unmarshal(in, obj)
}

// minEnv has two importable fields; Alpha is always deployed, Beta is optional.
type minEnv struct {
	Alpha *minImportable
	Beta  *minImportable
}

// testCtx is a minimal common.Context for standalone unit tests.
type testCtx struct{ dir string }

func newStandaloneTestCtx(t *testing.T) *testCtx { return &testCtx{dir: t.TempDir()} }
func (c *testCtx) T() *testing.T                 { return nil }
func (c *testCtx) Logf(f string, a ...any)       { log.Printf(f, a...) }
func (c *testCtx) FailNow(f string, a ...any)    { log.Fatalf(f, a...) }
func (c *testCtx) SessionOutputDir() string      { return c.dir }

var _ common.Context = (*testCtx)(nil)

// TestHydrateFromResources_KeyReplayRoundTrip is the critical regression guard:
// verifies that captured import keys are replayed correctly so that
// BuildEnvFromResources can match resources by export name, matching the
// behaviour that Provision relies on.
func TestHydrateFromResources_KeyReplayRoundTrip(t *testing.T) {
	// Simulate resources produced by Pulumi export, keyed by export name.
	resources := provisioners.RawResources{
		"dd-minImportable-alpha": mustMarshal(t, &minImportable{Value: "alpha-value"}),
	}

	// keys map mirrors what environments.ImportKeys would return after Provision.
	keys := map[string]string{
		"Alpha": "dd-minImportable-alpha",
		// Beta is absent — should end up nil in the hydrated env.
	}

	ctx := newStandaloneTestCtx(t)
	env, err := HydrateFromResources[minEnv](ctx, resources, keys)
	if err != nil {
		t.Fatalf("HydrateFromResources: %v", err)
	}

	// Alpha should be non-nil and have the imported value.
	if env.Alpha == nil {
		t.Fatal("Alpha should not be nil after hydration")
	}
	if env.Alpha.Value != "alpha-value" {
		t.Errorf("Alpha.Value: got %q, want %q", env.Alpha.Value, "alpha-value")
	}

	// Beta was absent from keys — it must be nil.
	if env.Beta != nil {
		t.Errorf("Beta should be nil (absent from keys), got %+v", env.Beta)
	}
}

// TestHydrateFromResources_MissingKeyIsNil verifies a field not in keys becomes nil.
func TestHydrateFromResources_MissingKeyIsNil(t *testing.T) {
	resources := provisioners.RawResources{}
	keys := map[string]string{} // no keys at all

	ctx := newStandaloneTestCtx(t)
	env, err := HydrateFromResources[minEnv](ctx, resources, keys)
	if err != nil {
		t.Fatalf("HydrateFromResources with empty keys: %v", err)
	}
	if env.Alpha != nil {
		t.Errorf("Alpha should be nil when absent from keys")
	}
	if env.Beta != nil {
		t.Errorf("Beta should be nil when absent from keys")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}
