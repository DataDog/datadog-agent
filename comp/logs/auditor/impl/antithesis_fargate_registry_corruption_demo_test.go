// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisFargateRegistryCorruption \
//	    ./comp/logs/auditor/impl/ -v
//
// Demonstrates property `registry-survives-crash`: the non-atomic registry writer
// (used when `atomic_registry_write=false`, the ECS Fargate default) does
// `os.Create(registryPath)` which TRUNCATES the existing file, then `f.Write(data)`.
// A crash between truncate and write leaves a zero-byte registry.json. On restart,
// recoverRegistry() reads the zero-byte file, json.Unmarshal fails, and returns an
// empty map — all tailers restart from their default position (mass loss/replay).
//
// The atomic writer uses temp-file + rename: the original file is intact until the
// rename completes, so a crash at any point leaves either the old-valid or new-valid
// file on disk.
//
// Evidence:
//   registry_writer.go:56-73  — nonAtomicRegistryWriter.WriteRegistry (os.Create truncates)
//   registry_writer.go:23-46  — atomicRegistryWriter.WriteRegistry (temp+rename)
//
// The test simulates "crash between truncate and write" for the non-atomic writer by
// injecting a writer that errors on Write() — exactly what happens if the process is
// killed after os.Create() returns but before Write() completes. We then assert:
//   1. The on-disk file is NEVER an invalid/empty state after a crash-sim write.
//      For the non-atomic writer this FAILS: the file is zero bytes.
//   2. The original valid content is still recoverable after the crash-sim.
//      For the non-atomic writer this FAILS: the truncation already happened.
//   3. The atomic writer passes both assertions: the original file is untouched.

package auditorimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// errSimulatedCrash is returned by the injected writer to simulate a mid-write crash.
var errSimulatedCrash = errors.New("simulated crash: process killed mid-write")

// crashingWriter wraps the non-atomic writer and simulates a crash after os.Create
// (truncation) but before Write completes. It does this by performing ONLY the
// truncation step (replicating exactly what nonAtomicRegistryWriter does) and then
// returning an error without writing any data.
type crashingNonAtomicWriter struct{}

// WriteRegistry reproduces the non-atomic writer's dangerous sequence but stops
// immediately after os.Create (which truncates the existing file), simulating a
// process kill before Write returns. This is the minimal faithful reproduction of
// the crash window described in registry_writer.go:56-73.
func (w *crashingNonAtomicWriter) WriteRegistry(registryPath string, _ string, _ string, _ []byte) error {
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return err
	}
	// os.Create truncates the existing file — this is the point of no return.
	// The original content is gone the instant this call returns.
	f, err := os.Create(registryPath) // registry_writer.go:63 — truncates!
	if err != nil {
		return err
	}
	f.Close()
	// Crash here: Write never happened. File is now zero bytes.
	return errSimulatedCrash
}

// crashingAtomicWriter simulates a crash inside the atomic write path — after
// CreateTemp and Write but before Rename. The temp file is orphaned; the
// original registry file is completely untouched.
type crashingAtomicWriter struct{}

func (w *crashingAtomicWriter) WriteRegistry(registryPath string, registryDirPath string, registryTmpFile string, data []byte) error {
	f, err := os.CreateTemp(registryDirPath, registryTmpFile)
	if err != nil {
		return err
	}
	tmpName := f.Name()
	// Write into the temp file (not the live registry).
	if _, err = f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpName)
		return err
	}
	f.Close()
	// Crash here: Rename never happened. The original registry file is untouched;
	// the temp file is orphaned but harmless.
	return errSimulatedCrash
}

// isValidRegistryJSON returns true if the file at path contains non-empty, parseable JSON.
func isValidRegistryJSON(path string) (bool, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, 0, fmt.Errorf("read error: %w", err)
	}
	size := int64(len(data))
	if size == 0 {
		return false, 0, nil
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return false, size, fmt.Errorf("json unmarshal error (size=%d): %w", size, err)
	}
	return true, size, nil
}

// TestAntithesisFargateRegistryCorruption is the main demonstration test.
//
// Structure:
//  1. Write a known-good registry payload via the *normal* (non-crashing) writer.
//  2. Simulate a crash during the next flush using the crashing writer variant.
//  3. Assert: the on-disk file is still the old valid content (crash-safe invariant).
//  4. Repeat for both the non-atomic and atomic writers.
//
// EXPECTED: non-atomic writer FAILS step 3 (file is zero bytes after crash-sim).
//
//	atomic writer PASSES step 3 (file unchanged by crash-sim).
func TestAntithesisFargateRegistryCorruption(t *testing.T) {
	t.Run("non_atomic_writer_crash_corrupts_registry", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, "registry.json")
		dirPath := dir
		tmpPrefix := "registry.json.tmp"

		// Step 1: Write a known-good registry.
		goodData := []byte(`{"source::/var/log/app.log":{"Identifier":"source::/var/log/app.log","Offset":"12345","TailingMode":"beginning","LastUpdated":"2024-01-01T00:00:00Z"}}`)
		realWriter := NewNonAtomicRegistryWriter()
		if err := realWriter.WriteRegistry(registryPath, dirPath, tmpPrefix, goodData); err != nil {
			t.Fatalf("initial write failed: %v", err)
		}

		// Verify the initial state is good.
		valid, size, err := isValidRegistryJSON(registryPath)
		if err != nil || !valid {
			t.Fatalf("pre-crash registry is not valid (size=%d, err=%v)", size, err)
		}
		t.Logf("pre-crash registry: valid=true, size=%d bytes", size)

		// Step 2: Simulate a crash during the next flush (non-atomic path).
		// The crashingNonAtomicWriter replicates the exact sequence:
		//   os.Create(registryPath) → truncate → crash (no Write)
		crashWriter := &crashingNonAtomicWriter{}
		newData := []byte(`{"source::/var/log/app.log":{"Identifier":"source::/var/log/app.log","Offset":"99999","TailingMode":"beginning","LastUpdated":"2024-01-01T00:00:01Z"}}`)
		crashErr := crashWriter.WriteRegistry(registryPath, dirPath, tmpPrefix, newData)
		if !errors.Is(crashErr, errSimulatedCrash) {
			t.Fatalf("expected simulated crash error, got: %v", crashErr)
		}
		t.Logf("crash injected (os.Create truncated; Write never called)")

		// Step 3: Assert crash-safety invariant:
		//   the file must be either old-valid or new-valid — NEVER zero/corrupt.
		valid, size, readErr := isValidRegistryJSON(registryPath)
		t.Logf("post-crash registry: valid=%v, size=%d bytes, read_err=%v", valid, size, readErr)

		if !valid {
			// BUG DEMONSTRATED: the non-atomic writer truncated the file before
			// writing, so a crash between os.Create and Write leaves zero bytes.
			// recoverRegistry() will return an empty map on next startup.
			t.Fatalf(
				"BUG DEMONSTRATED (registry-survives-crash / non-atomic writer):\n"+
					"  os.Create(registryPath) at registry_writer.go:63 truncated the file.\n"+
					"  The simulated crash (no Write) left the file with %d bytes.\n"+
					"  On agent restart, recoverRegistry() returns an empty map.\n"+
					"  All tailers restart from default position → mass loss/replay.\n"+
					"  File path: %s\n"+
					"  Read error (if any): %v",
				size, registryPath, readErr,
			)
		}
	})

	t.Run("atomic_writer_crash_leaves_registry_intact", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, "registry.json")
		dirPath := dir
		tmpPrefix := "registry.json.tmp"

		// Step 1: Write a known-good registry.
		goodData := []byte(`{"source::/var/log/app.log":{"Identifier":"source::/var/log/app.log","Offset":"12345","TailingMode":"beginning","LastUpdated":"2024-01-01T00:00:00Z"}}`)
		realWriter := NewAtomicRegistryWriter()
		if err := realWriter.WriteRegistry(registryPath, dirPath, tmpPrefix, goodData); err != nil {
			t.Fatalf("initial write failed: %v", err)
		}

		valid, size, err := isValidRegistryJSON(registryPath)
		if err != nil || !valid {
			t.Fatalf("pre-crash registry is not valid (size=%d, err=%v)", size, err)
		}
		t.Logf("pre-crash registry: valid=true, size=%d bytes", size)

		// Step 2: Simulate a crash during the next flush (atomic path).
		// The crashingAtomicWriter writes to a temp file then "crashes" before Rename.
		// The original registry file is NEVER touched.
		crashWriter := &crashingAtomicWriter{}
		newData := []byte(`{"source::/var/log/app.log":{"Identifier":"source::/var/log/app.log","Offset":"99999","TailingMode":"beginning","LastUpdated":"2024-01-01T00:00:01Z"}}`)
		crashErr := crashWriter.WriteRegistry(registryPath, dirPath, tmpPrefix, newData)
		if !errors.Is(crashErr, errSimulatedCrash) {
			t.Fatalf("expected simulated crash error, got: %v", crashErr)
		}
		t.Logf("crash injected (temp file written; Rename never called)")

		// Step 3: Assert crash-safety invariant: original file must be intact.
		valid, size, readErr := isValidRegistryJSON(registryPath)
		t.Logf("post-crash registry: valid=%v, size=%d bytes, read_err=%v", valid, size, readErr)

		if !valid {
			t.Fatalf(
				"UNEXPECTED: atomic writer crash left registry invalid (size=%d, err=%v)",
				size, readErr,
			)
		}

		// Also verify the content is still the original good data (not new or empty).
		content, _ := os.ReadFile(registryPath)
		if string(content) != string(goodData) {
			t.Fatalf(
				"UNEXPECTED: atomic writer crash changed registry content.\n  got: %s\n  want: %s",
				content, goodData,
			)
		}
		t.Logf("atomic writer: original content preserved after crash — crash-safe invariant HOLDS")
	})
}
