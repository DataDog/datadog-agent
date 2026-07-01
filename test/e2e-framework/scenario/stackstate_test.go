// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"errors"
	"testing"
)

func TestStackConfigRoundTrip(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	want := map[string]string{
		"os":   "ubuntu-22.04",
		"arch": "arm64",
	}

	if err := SaveStackConfig("my-stack", want); err != nil {
		t.Fatalf("SaveStackConfig: %v", err)
	}

	got, err := LoadStackConfig("my-stack")
	if err != nil {
		t.Fatalf("LoadStackConfig: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("round-trip length mismatch: got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestStackConfigNotFound(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	_, err := LoadStackConfig("nonexistent-stack")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNoStackConfig) {
		t.Fatalf("expected ErrNoStackConfig, got %v", err)
	}
}

func TestStackConfigDelete(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	if err := SaveStackConfig("del-stack", map[string]string{"os": "debian-12"}); err != nil {
		t.Fatalf("SaveStackConfig: %v", err)
	}
	if err := DeleteStackConfig("del-stack"); err != nil {
		t.Fatalf("DeleteStackConfig: %v", err)
	}
	_, err := LoadStackConfig("del-stack")
	if !errors.Is(err, ErrNoStackConfig) {
		t.Fatalf("expected ErrNoStackConfig after delete, got %v", err)
	}
}

func TestStackConfigDeleteNoOp(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())
	// Deleting a non-existent file must not error
	if err := DeleteStackConfig("never-existed"); err != nil {
		t.Fatalf("DeleteStackConfig on absent file: %v", err)
	}
}
