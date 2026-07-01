// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func seedStack(t *testing.T, stack, scenarioName string) ProvisionedStack {
	t.Helper()
	ps := ProvisionedStack{
		Scenario: scenarioName,
		Stack:    stack,
		Config:   map[string]string{"region": "us-east-1"},
		Resources: map[string]json.RawMessage{
			"host": json.RawMessage(`{"ip":"10.0.0.1"}`),
		},
		Keys: map[string]string{
			"RemoteHost": "dd-Host-main",
		},
		CreatedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := SaveProvisionedStack(ps); err != nil {
		t.Fatalf("SaveProvisionedStack: %v", err)
	}
	return ps
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	want := seedStack(t, "my-stack", "ec2-host")

	got, err := LoadProvisionedStack("my-stack")
	if err != nil {
		t.Fatalf("LoadProvisionedStack: %v", err)
	}

	if got.Scenario != want.Scenario {
		t.Errorf("Scenario: got %q, want %q", got.Scenario, want.Scenario)
	}
	if got.Stack != want.Stack {
		t.Errorf("Stack: got %q, want %q", got.Stack, want.Stack)
	}
	if got.Config["region"] != want.Config["region"] {
		t.Errorf("Config[region]: got %q, want %q", got.Config["region"], want.Config["region"])
	}
	if string(got.Resources["host"]) != string(want.Resources["host"]) {
		t.Errorf("Resources[host]: got %s, want %s", got.Resources["host"], want.Resources["host"])
	}
	if got.Keys["RemoteHost"] != want.Keys["RemoteHost"] {
		t.Errorf("Keys[RemoteHost]: got %q, want %q", got.Keys["RemoteHost"], want.Keys["RemoteHost"])
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestLoadMissingReturnsErrNoProvisionedStack(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	_, err := LoadProvisionedStack("does-not-exist")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrNoProvisionedStack) {
		t.Errorf("expected ErrNoProvisionedStack, got: %v", err)
	}
}

func TestListReturnsSortedStacks(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	seedStack(t, "z-stack", "scenario-z")
	seedStack(t, "a-stack", "scenario-a")
	seedStack(t, "m-stack", "scenario-m")

	stacks, err := ListProvisionedStacks()
	if err != nil {
		t.Fatalf("ListProvisionedStacks: %v", err)
	}
	if len(stacks) != 3 {
		t.Fatalf("expected 3 stacks, got %d", len(stacks))
	}
	names := []string{stacks[0].Stack, stacks[1].Stack, stacks[2].Stack}
	want := []string{"a-stack", "m-stack", "z-stack"}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("stacks[%d].Stack = %q, want %q", i, n, want[i])
		}
	}
}

func TestListEmptyDirReturnsNil(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	stacks, err := ListProvisionedStacks()
	if err != nil {
		t.Fatalf("ListProvisionedStacks: %v", err)
	}
	if len(stacks) != 0 {
		t.Errorf("expected empty slice, got %v", stacks)
	}
}

func TestListAbsentDirReturnsNil(t *testing.T) {
	// Point to a directory that does not exist.
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir()+"/nonexistent")

	stacks, err := ListProvisionedStacks()
	if err != nil {
		t.Fatalf("ListProvisionedStacks: %v", err)
	}
	if len(stacks) != 0 {
		t.Errorf("expected empty slice, got %v", stacks)
	}
}

func TestDeleteRemovesStack(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	seedStack(t, "del-stack", "ec2-host")

	if err := DeleteProvisionedStack("del-stack"); err != nil {
		t.Fatalf("DeleteProvisionedStack: %v", err)
	}

	_, err := LoadProvisionedStack("del-stack")
	if !errors.Is(err, ErrNoProvisionedStack) {
		t.Errorf("expected ErrNoProvisionedStack after delete, got: %v", err)
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	if err := DeleteProvisionedStack("never-existed"); err != nil {
		t.Fatalf("DeleteProvisionedStack on missing stack should not error: %v", err)
	}
}

func TestToFromRawMessage(t *testing.T) {
	raw := map[string]json.RawMessage{
		"key": json.RawMessage(`{"val":42}`),
	}
	res := fromRawMessage(raw)
	if string(res["key"]) != `{"val":42}` {
		t.Errorf("fromRawMessage: got %s", res["key"])
	}
	back := toRawMessage(res)
	if string(back["key"]) != `{"val":42}` {
		t.Errorf("toRawMessage round-trip: got %s", back["key"])
	}
}
