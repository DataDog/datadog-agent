// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis migration-safety demonstration for property `registry-format-migration-safe`,
// gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisRegistryFormatMigrationSafe \
//	    ./comp/logs/auditor/impl/ -v
//
// Claim (from registry-format-migration-safe.md):
// Migrating a v0 or v1 registry through the current auditor should preserve all
// offset entries with valid data. The `default` branch of unmarshalRegistry()
// (auditor.go:458-461) should never fire for known versions (0, 1, 2).
//
// Key code path:
//
//	unmarshalRegistry() auditor.go:440-462 — version dispatch
//	unmarshalRegistryV0() api_v0.go:27-45  — v0 migration (adds "file:" prefix)
//	unmarshalRegistryV1() api_v1.go:27-44  — v1 migration (Offset int64 or Timestamp)
//	unmarshalRegistryV2() api_v2.go:15-27  — v2 direct unmarshal
//
// Test structure:
//  1. Sub-test "v1_migration_preserves_offsets": write a v1 registry with two
//     entries (one with Offset>0, one with Timestamp string), recover, assert
//     both entries are present with correct offsets. EXPECTED: PASS (migration correct).
//  2. Sub-test "v0_migration_preserves_offsets": write a v0 registry with two
//     file entries (both Offset>0), recover, assert identifiers have "file:" prefix
//     and correct offsets. EXPECTED: PASS.
//  3. Sub-test "v0_drops_zero_offset_entries": write a v0 registry entry with
//     Offset=0 — api_v0.go:35 explicitly drops these. Assert the entry is absent.
//     VERDICT for this sub-test: the behavior is intentional (zero means no data
//     yet), but we document it to surface the implicit data model assumption.
//  4. Sub-test "unknown_version_returns_error": write a version=99 registry,
//     assert recoverRegistry() returns empty map (the default branch fires).
//     This is correct behavior but we verify the default branch is reachable.
//  5. Sub-test "v2_round_trip": the current write version; flush + recover preserves
//     all fields including TailingMode and Fingerprint. EXPECTED: PASS.

package auditorimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// newMigrationTestAuditor builds a registryAuditor pointed at dir.
func newMigrationTestAuditor(t *testing.T, dir string) *registryAuditor {
	t.Helper()
	cfg := configmock.NewMock(t)
	cfg.SetWithoutSource("logs_config.run_path", dir)
	return newAuditor(Dependencies{
		Config:     cfg,
		Log:        logmock.New(t),
		KubeHealth: kubehealthmock.NewMockRegistrar(),
	})
}

// writeRegistryFile marshals v and writes it as the registry.json in dir.
func writeRegistryFile(t *testing.T, dir string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	p := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
	t.Logf("wrote registry.json (%d bytes): %s", len(data), string(data))
}

// TestAntithesisRegistryFormatMigrationSafe tests that version migration preserves
// all valid entries and that the version dispatch behaves as documented.
func TestAntithesisRegistryFormatMigrationSafe(t *testing.T) {

	// -----------------------------------------------------------------------
	// Sub-test 1: v1 migration preserves both Offset and Timestamp entries
	//
	// v1 format has two offset representations:
	//   - entry.Offset int64 > 0   → stored as decimal string
	//   - entry.Timestamp string != "" → stored verbatim as Offset string
	// Both paths must survive migration (api_v1.go:35-41).
	// -----------------------------------------------------------------------
	t.Run("v1_migration_preserves_offsets", func(t *testing.T) {
		dir := t.TempDir()

		v1Registry := struct {
			Version  int
			Registry map[string]registryEntryV1
		}{
			Version: 1,
			Registry: map[string]registryEntryV1{
				"file:/var/log/app.log": {
					Offset:      12345,
					LastUpdated: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				"docker://container-abc": {
					Timestamp:   "2024-01-01T12:00:00.000000001Z",
					LastUpdated: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				},
			},
		}
		writeRegistryFile(t, dir, v1Registry)

		a := newMigrationTestAuditor(t, dir)
		a.registry = a.recoverRegistry()

		// Verify file tailer offset preserved.
		fileOffset := a.GetOffset("file:/var/log/app.log")
		containerOffset := a.GetOffset("docker://container-abc")

		t.Logf("v1 migration results:")
		t.Logf("  file:/var/log/app.log offset = %q (want \"12345\")", fileOffset)
		t.Logf("  docker://container-abc offset = %q (want \"2024-01-01T12:00:00.000000001Z\")", containerOffset)

		failed := false
		if fileOffset != "12345" {
			t.Errorf("REPRODUCED: v1 migration dropped file offset: got %q, want \"12345\"", fileOffset)
			failed = true
		}
		if containerOffset != "2024-01-01T12:00:00.000000001Z" {
			t.Errorf("REPRODUCED: v1 migration dropped container timestamp: got %q, want \"2024-01-01T12:00:00.000000001Z\"", containerOffset)
			failed = true
		}
		if !failed {
			t.Logf("REFUTED: v1 migration correctly preserved both entry types — migration is safe for v1")
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 2: v0 migration rewrites identifiers with "file:" prefix
	//
	// api_v0.go:38: newIdentifier = "file:" + identifier
	// Both entries have Offset>0 so both should survive.
	// -----------------------------------------------------------------------
	t.Run("v0_migration_adds_file_prefix", func(t *testing.T) {
		dir := t.TempDir()

		v0Registry := struct {
			Version  int
			Registry map[string]registryEntryV0
		}{
			Version: 0,
			Registry: map[string]registryEntryV0{
				"/var/log/app.log":    {Path: "/var/log/app.log", Offset: 1000, Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				"/var/log/other.log":  {Path: "/var/log/other.log", Offset: 2000, Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
		}
		writeRegistryFile(t, dir, v0Registry)

		a := newMigrationTestAuditor(t, dir)
		a.registry = a.recoverRegistry()

		t.Logf("v0 migration results:")
		for _, id := range []string{"file:/var/log/app.log", "file:/var/log/other.log"} {
			t.Logf("  %s offset = %q", id, a.GetOffset(id))
		}
		// Old identifiers should NOT exist (they got the file: prefix).
		for _, id := range []string{"/var/log/app.log", "/var/log/other.log"} {
			t.Logf("  %s (old, no prefix) offset = %q", id, a.GetOffset(id))
		}

		failed := false
		if a.GetOffset("file:/var/log/app.log") != "1000" {
			t.Errorf("REPRODUCED: v0 migration dropped or misidentified app.log: got %q, want \"1000\"",
				a.GetOffset("file:/var/log/app.log"))
			failed = true
		}
		if a.GetOffset("file:/var/log/other.log") != "2000" {
			t.Errorf("REPRODUCED: v0 migration dropped or misidentified other.log: got %q, want \"2000\"",
				a.GetOffset("file:/var/log/other.log"))
			failed = true
		}
		// Old identifiers must be absent after prefix rewrite.
		if a.GetOffset("/var/log/app.log") != "" {
			t.Errorf("REPRODUCED: v0 migration kept old non-prefixed identifier: %q should be gone",
				"/var/log/app.log")
			failed = true
		}
		if !failed {
			t.Logf("REFUTED: v0 migration correctly added file: prefix and preserved offsets — migration is safe for v0")
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 3: v0 drops entries with Offset=0
	//
	// api_v0.go:35-42: the switch only saves entries where entry.Offset > 0.
	// An entry with Offset=0 is silently dropped. This is intentional — zero
	// means "no data read yet" in v0 — but we document the behavior to confirm
	// that zero-offset entries from v0 are never migrated.
	// -----------------------------------------------------------------------
	t.Run("v0_drops_zero_offset_entries", func(t *testing.T) {
		dir := t.TempDir()

		v0Registry := struct {
			Version  int
			Registry map[string]registryEntryV0
		}{
			Version: 0,
			Registry: map[string]registryEntryV0{
				"/var/log/app.log":    {Path: "/var/log/app.log", Offset: 500},  // kept
				"/var/log/zero.log":   {Path: "/var/log/zero.log", Offset: 0},   // dropped by api_v0.go:35
			},
		}
		writeRegistryFile(t, dir, v0Registry)

		a := newMigrationTestAuditor(t, dir)
		a.registry = a.recoverRegistry()

		appOffset := a.GetOffset("file:/var/log/app.log")
		zeroOffset := a.GetOffset("file:/var/log/zero.log")

		t.Logf("v0 zero-offset drop behavior:")
		t.Logf("  file:/var/log/app.log (offset=500): got %q (want \"500\")", appOffset)
		t.Logf("  file:/var/log/zero.log (offset=0):  got %q (want \"\" — intentionally dropped by api_v0.go:35)", zeroOffset)

		if appOffset != "500" {
			t.Errorf("REPRODUCED unexpected: non-zero offset entry was dropped: got %q, want \"500\"", appOffset)
		}
		if zeroOffset != "" {
			t.Errorf("REPRODUCED unexpected: zero-offset entry was kept (should be dropped by api_v0.go:35): got %q", zeroOffset)
		} else {
			t.Logf("CONFIRMED: v0 zero-offset drop is intentional and works as documented (api_v0.go:35)")
			t.Logf("NOTE: any tailer whose first write was interrupted (Offset=0) loses its position on v0→v2 upgrade")
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 4: unknown version returns error → empty map
	//
	// auditor.go:458-461: default branch returns errors.New("invalid registry version number")
	// recoverRegistry() (auditor.go:347-352) treats this as corrupt → empty map.
	// -----------------------------------------------------------------------
	t.Run("unknown_version_returns_empty_map", func(t *testing.T) {
		dir := t.TempDir()

		unknownVersionRegistry := struct {
			Version  int
			Registry map[string]RegistryEntry
		}{
			Version: 99,
			Registry: map[string]RegistryEntry{
				"file:/var/log/app.log": {Offset: "12345", TailingMode: "beginning"},
			},
		}
		writeRegistryFile(t, dir, unknownVersionRegistry)

		a := newMigrationTestAuditor(t, dir)
		a.registry = a.recoverRegistry()

		offset := a.GetOffset("file:/var/log/app.log")
		t.Logf("unknown version=99 recovery result: GetOffset = %q", offset)

		if offset != "" {
			t.Errorf("UNEXPECTED: version=99 registry was somehow recovered with offset %q", offset)
		} else {
			// This is the CORRECT/expected behavior — unknown versions should error.
			// But we document it: the default branch at auditor.go:458-461 fires.
			// If a future v3 agent writes version=3, a rollback to the current agent
			// hits this default branch and loses all offsets.
			t.Logf("CONFIRMED: unknown version=99 correctly returns empty map (auditor.go:458-461 default branch)")
			t.Logf("NOTE: rollback from a future v3 agent to this agent would silently lose all offsets")
		}
	})

	// -----------------------------------------------------------------------
	// Sub-test 5: v2 round-trip preserves all fields including TailingMode + Fingerprint
	//
	// This is the happy path for the current version — verify it as a baseline.
	// -----------------------------------------------------------------------
	t.Run("v2_round_trip_preserves_all_fields", func(t *testing.T) {
		dir := t.TempDir()
		const id = "file:/var/log/app.log"

		fpConfig := &types.FingerprintConfig{
			FingerprintStrategy: types.FingerprintStrategyLineChecksum,
			Count:               1,
		}
		a := newMigrationTestAuditor(t, dir)
		a.registry = map[string]*RegistryEntry{
			id: {
				LastUpdated:        time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				Offset:             "88888",
				TailingMode:        "beginning",
				IngestionTimestamp: 42,
				Fingerprint:        types.Fingerprint{Value: 99999, Config: fpConfig},
			},
		}
		if err := a.flushRegistry(); err != nil {
			t.Fatalf("flush failed: %v", err)
		}

		a2 := newMigrationTestAuditor(t, dir)
		a2.registry = a2.recoverRegistry()

		offset := a2.GetOffset(id)
		mode := a2.GetTailingMode(id)
		fp := a2.GetFingerprint(id)

		t.Logf("v2 round-trip: offset=%q mode=%q fp.Value=%v", offset, mode, fp)

		failed := false
		if offset != "88888" {
			t.Errorf("REPRODUCED: v2 round-trip lost offset: got %q, want \"88888\"", offset)
			failed = true
		}
		if mode != "beginning" {
			t.Errorf("REPRODUCED: v2 round-trip lost TailingMode: got %q, want \"beginning\"", mode)
			failed = true
		}
		if fp == nil || fp.Value != 99999 {
			t.Errorf("REPRODUCED: v2 round-trip lost Fingerprint: got %v", fp)
			failed = true
		}
		if !failed {
			t.Logf("REFUTED: v2 round-trip correctly preserved all fields — v2 migration is safe")
		}
	})
}
