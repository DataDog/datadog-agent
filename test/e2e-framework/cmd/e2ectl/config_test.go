// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
)

func TestCandidateValidationDoesNotReplaceStoredConfig(t *testing.T) {
	entry := envstore.Entry{Dir: t.TempDir(), Meta: envstore.Meta{Base: "kind"}}
	original := []byte("original stored config")
	if err := os.WriteFile(entry.ConfigPath(), original, 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate.yaml")
	invalid := []byte("schema: 1\nenvironment:\n  base: kind\n  kind:\n    nodes: -1\nagent:\n  install: helm\n")
	if err := os.WriteFile(candidate, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrStoredConfig(candidate, entry); err == nil {
		t.Fatal("driver schema validation should reject the candidate")
	}
	got, err := os.ReadFile(entry.ConfigPath())
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("invalid candidate replaced stored state: %s %v", got, err)
	}
}

func TestExistingInfrastructureComparisonUsesTypedValues(t *testing.T) {
	const original = `schema: 1
environment:
  base: kind
  kind:
    nodes: 0 # original comment
agent:
  install: helm
  helm:
    version: 7.83.0
`
	for _, tt := range []struct {
		name        string
		environment string
		wantError   bool
	}{
		{"different comments", "  base: kind\n  kind:\n    nodes: 0 # new comment\n", false},
		{"explicit fixture default", "  fakeintake: true\n  kind: {nodes: 0}\n  base: kind\n", false},
		{"omitted driver default", "  base: kind\n  kind: {}\n", false},
		{"different worker count", "  base: kind\n  kind: {nodes: 1}\n", true},
		{"different fixture intent", "  base: kind\n  fakeintake: false\n  kind: {}\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entry := envstore.Entry{Dir: t.TempDir(), Meta: envstore.Meta{Base: "kind"}}
			if err := os.WriteFile(entry.ConfigPath(), []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			candidate := filepath.Join(t.TempDir(), "candidate.yaml")
			// Changing the Agent version is permitted; infrastructure must match.
			data := "schema: 1\nenvironment:\n" + tt.environment + "agent:\n  install: helm\n  helm:\n    version: 7.69.0\n"
			if err := os.WriteFile(candidate, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadOrStoredConfig(candidate, entry)
			if (err != nil) != tt.wantError {
				t.Fatalf("want validation error=%v, got %v", tt.wantError, err)
			}
			stored, err := os.ReadFile(entry.ConfigPath())
			if err != nil || string(stored) != original {
				t.Fatalf("validation changed stored config: %v", err)
			}
		})
	}
}

func TestSaveAppliedConfigUsesPreparedSource(t *testing.T) {
	entry := envstore.Entry{Dir: t.TempDir(), Meta: envstore.Meta{Base: "kind"}}
	source, err := driver.StarterConfig("kind")
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate.yaml")
	if err := os.WriteFile(candidate, source, 0o600); err != nil {
		t.Fatal(err)
	}
	// A live entry has a stored provisioning config; candidate installs may not change it.
	if err := os.WriteFile(entry.ConfigPath(), source, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadOrStoredConfig(candidate, entry)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an operator replacing/removing the source during a long install.
	if err := os.Remove(candidate); err != nil {
		t.Fatal(err)
	}
	if err := saveAppliedConfig(cfg, entry); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(entry.ConfigPath())
	if err != nil || !bytes.Equal(got, source) {
		t.Fatalf("did not persist the source actually parsed: %v", err)
	}
	info, err := os.Stat(entry.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config must be private, got %v", info.Mode())
	}
}
