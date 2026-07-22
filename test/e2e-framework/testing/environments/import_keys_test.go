// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package environments

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

// testImportable is a minimal Importable used only in tests.
type testImportable struct {
	components.JSONImporter
	Value string `json:"value"`
}

func (t *testImportable) Import(in []byte, obj any) error {
	return json.Unmarshal(in, obj)
}

// testEnv has two importable pointer fields and one non-importable field.
type testEnv struct {
	Alpha   *testImportable
	Beta    *testImportable
	Ignored int // not importable — should never appear in ImportKeys output
}

func TestImportKeys_SnapshotsNonEmptyKeys(t *testing.T) {
	env := &testEnv{
		Alpha: &testImportable{},
		Beta:  &testImportable{},
	}
	env.Alpha.SetKey("dd-Alpha-main")
	env.Beta.SetKey("dd-Beta-main")

	got := ImportKeys(env)

	if got["Alpha"] != "dd-Alpha-main" {
		t.Errorf("Alpha key: got %q, want %q", got["Alpha"], "dd-Alpha-main")
	}
	if got["Beta"] != "dd-Beta-main" {
		t.Errorf("Beta key: got %q, want %q", got["Beta"], "dd-Beta-main")
	}
	if _, ok := got["Ignored"]; ok {
		t.Errorf("Ignored field should not appear in ImportKeys output")
	}
}

func TestImportKeys_SkipsNilFields(t *testing.T) {
	env := &testEnv{
		Alpha: &testImportable{},
		Beta:  nil,
	}
	env.Alpha.SetKey("dd-Alpha-main")

	got := ImportKeys(env)

	if got["Alpha"] != "dd-Alpha-main" {
		t.Errorf("Alpha key: got %q, want %q", got["Alpha"], "dd-Alpha-main")
	}
	if _, ok := got["Beta"]; ok {
		t.Errorf("nil Beta field should not appear in ImportKeys output")
	}
}

func TestImportKeys_SkipsEmptyKeys(t *testing.T) {
	env := &testEnv{
		Alpha: &testImportable{}, // key not set — Key() returns ""
		Beta:  &testImportable{},
	}
	env.Beta.SetKey("dd-Beta-main")

	got := ImportKeys(env)

	if _, ok := got["Alpha"]; ok {
		t.Errorf("field with empty key should not appear in ImportKeys output")
	}
	if got["Beta"] != "dd-Beta-main" {
		t.Errorf("Beta key: got %q, want %q", got["Beta"], "dd-Beta-main")
	}
}

func TestImportKeys_NilEnvReturnsEmpty(t *testing.T) {
	got := ImportKeys(nil)
	if len(got) != 0 {
		t.Errorf("expected empty map for nil env, got %v", got)
	}
}
