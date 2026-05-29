// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* for property `registry-recovers-after-crash`,
// gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisRegistryRecoversAfterCrash \
//	    ./comp/logs/auditor/impl/ -v
//
// Claim (from registry-recovers-after-crash.md):
// If registry.json is MISSING or contains CORRUPT/UNPARSEABLE data,
// recoverRegistry() (auditor.go:337-353) silently returns an empty map with no
// error surfaced to the caller. On the next Start(), every tailer resumes from
// its default position (usually end-of-file) causing silent data loss, or
// beginning-of-file causing mass replay.
//
// Key code path:
//
//	recoverRegistry() auditor.go:337-353
//	  - os.IsNotExist(err) → log.Info, return make(map[string]*RegistryEntry)
//	  - unmarshalRegistry error → log.Error, return make(map[string]*RegistryEntry)
//	  - No error is returned; caller (Start) has no signal that recovery failed.
//
// Test structure:
//  1. Sub-test "missing_registry": no file on disk at all → recoverRegistry
//     returns empty map → GetOffset("known-id") == "" → REPRODUCED (silent loss).
//  2. Sub-test "corrupt_registry": write a zero-byte file (the exact artifact left
//     by a non-atomic writer crash) → recoverRegistry returns empty map →
//     GetOffset("known-id") == "" → REPRODUCED (silent loss).
//  3. Sub-test "corrupt_json": write syntactically invalid JSON → same outcome.
//
// VERDICT expectation: REPRODUCED for all three sub-tests — the auditor silently
// falls back to an empty registry without surfacing an error.

package auditorimpl

import (
	"os"
	"path/filepath"
	"testing"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
)

// newTestAuditorForPath builds a registryAuditor pointed at the given directory,
// using mock dependencies. Does NOT call Start() — tests exercise recoverRegistry()
// directly, as Start() does (auditor.go:127).
func newTestAuditorForPath(t *testing.T, runPath string) *registryAuditor {
	t.Helper()
	cfg := configmock.NewMock(t)
	cfg.SetWithoutSource("logs_config.run_path", runPath)
	deps := Dependencies{
		Config:     cfg,
		Log:        logmock.New(t),
		KubeHealth: kubehealthmock.NewMockRegistrar(),
	}
	return newAuditor(deps)
}

// TestAntithesisRegistryRecoversAfterCrash demonstrates Bug 1:
// recoverRegistry() silently returns an empty map for missing and corrupt
// registry files, causing all tailers to resume from their default position.
func TestAntithesisRegistryRecoversAfterCrash(t *testing.T) {
	// The identifier that a tailer would normally look up on restart.
	const knownID = "file:/var/log/app.log"
	const knownOffset = "99999"

	// -----------------------------------------------------------------------
	// Sub-test 1: registry.json does not exist at all
	// This is the first-boot case but also the case after atomic rename completes
	// on a brand-new volume, or after manual deletion.
	// -----------------------------------------------------------------------
	t.Run("missing_registry", func(t *testing.T) {
		dir := t.TempDir()
		// Confirm there is no registry file.
		registryPath := filepath.Join(dir, "registry.json")
		if _, err := os.Stat(registryPath); !os.IsNotExist(err) {
			t.Fatalf("precondition failed: registry.json should not exist, stat err: %v", err)
		}

		a := newTestAuditorForPath(t, dir)
		// recoverRegistry() is what Start() calls (auditor.go:127).
		a.registry = a.recoverRegistry()

		offset := a.GetOffset(knownID)
		if offset != "" {
			// Unexpected: somehow returned an offset without a file — should never happen.
			t.Logf("UNEXPECTED: GetOffset returned %q for missing registry — not empty", offset)
		} else {
			// BUG DEMONSTRATED: the auditor silently recovered with an empty map.
			// No error was returned to the caller; Start() proceeds normally.
			// Every tailer will resume from its TailingMode default position.
			t.Logf("BUG DEMONSTRATED (registry-recovers-after-crash / missing registry):")
			t.Logf("  recoverRegistry() silently returned empty map (auditor.go:337-353)")
			t.Logf("  GetOffset(%q) == \"\" — tailer resumes from default TailingMode position", knownID)
			t.Logf("  No error surfaced; Start() proceeds as if this is a clean first boot")
			t.Fatalf(
				"REPRODUCED: recoverRegistry() silently returned empty map for missing registry.\n"+
					"  Identifier %q has no offset after recovery.\n"+
					"  Tailers will restart from TailingMode default (end-of-file=loss, beginning=replay).\n"+
					"  auditor.go:337-353 — no error returned, only a log.Info line.",
				knownID,
			)
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 2: registry.json is zero bytes
	// This is the exact artifact of a non-atomic writer crash (os.Create truncates,
	// then the process is killed before Write completes).
	// See registry_writer.go:63 and antithesis_fargate_registry_corruption_demo_test.go.
	// -----------------------------------------------------------------------
	t.Run("zero_byte_registry", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, "registry.json")

		// Write a valid registry first (simulates the pre-crash state).
		a := newTestAuditorForPath(t, dir)
		a.registry = map[string]*RegistryEntry{
			knownID: {Offset: knownOffset, TailingMode: "beginning"},
		}
		if err := a.flushRegistry(); err != nil {
			t.Fatalf("failed to write pre-crash registry: %v", err)
		}

		// Simulate the crash artifact: truncate to zero bytes.
		f, err := os.Create(registryPath)
		if err != nil {
			t.Fatalf("failed to truncate registry to zero bytes: %v", err)
		}
		f.Close()
		info, _ := os.Stat(registryPath)
		t.Logf("registry.json is now %d bytes (simulating crash artifact)", info.Size())

		// Now simulate agent restart: new auditor, same path, call recoverRegistry().
		a2 := newTestAuditorForPath(t, dir)
		a2.registry = a2.recoverRegistry()

		offset := a2.GetOffset(knownID)
		if offset == knownOffset {
			t.Logf("REFUTED: GetOffset returned the correct offset %q — recovery preserved state", offset)
		} else {
			t.Logf("BUG DEMONSTRATED (registry-recovers-after-crash / zero-byte registry):")
			t.Logf("  Zero-byte file was written by simulated crash (os.Create truncate, no Write)")
			t.Logf("  recoverRegistry() (auditor.go:337-353) got json.Unmarshal error → empty map")
			t.Logf("  GetOffset(%q) == %q (want %q)", knownID, offset, knownOffset)
			t.Fatalf(
				"REPRODUCED: recoverRegistry() silently returned empty map for zero-byte registry.\n"+
					"  Identifier %q: got offset %q, want %q.\n"+
					"  auditor.go:347-352 — unmarshal error logs and returns empty map, no error to caller.\n"+
					"  Tailers restart from TailingMode default → silent data loss or mass replay.",
				knownID, offset, knownOffset,
			)
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 3: registry.json contains syntactically invalid JSON
	// This covers partial writes, bit-rot, and filesystem corruption.
	// -----------------------------------------------------------------------
	t.Run("corrupt_json_registry", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, "registry.json")

		// Write corrupt JSON directly.
		corrupt := []byte(`{"Version":2,"Registry":{"file:/var/log/app.log":CORRUPT}}`)
		if err := os.WriteFile(registryPath, corrupt, 0644); err != nil {
			t.Fatalf("failed to write corrupt registry: %v", err)
		}

		a := newTestAuditorForPath(t, dir)
		a.registry = a.recoverRegistry()

		offset := a.GetOffset(knownID)
		if offset != "" {
			t.Logf("UNEXPECTED: GetOffset returned %q for corrupt registry", offset)
		} else {
			t.Logf("BUG DEMONSTRATED (registry-recovers-after-crash / corrupt JSON):")
			t.Logf("  Corrupt JSON in registry.json → json.Unmarshal fails → empty map (auditor.go:347-352)")
			t.Logf("  GetOffset(%q) == \"\" — tailer resumes from default TailingMode position", knownID)
			t.Fatalf(
				"REPRODUCED: recoverRegistry() silently returned empty map for corrupt JSON registry.\n"+
					"  Identifier %q has no offset after recovery.\n"+
					"  auditor.go:347-352 — unmarshal error logged but not surfaced; Start() continues.",
				knownID,
			)
		}
	})
}
