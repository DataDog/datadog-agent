// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package integrations contains packaging logic for python integrations
package integrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyOCIBaseline(t *testing.T) {
	const legacyContent = "datadog-checks-base==1.0.0\ndatadog-ping==1.0.2\n"

	t.Run("legacy baseline is migrated to storage when absent", func(t *testing.T) {
		legacyDir := t.TempDir()
		storageDir := filepath.Join(t.TempDir(), "run") // not yet created
		if err := os.WriteFile(filepath.Join(legacyDir, baselineFileName), []byte(legacyContent), 0o644); err != nil {
			t.Fatalf("failed to seed legacy baseline: %v", err)
		}

		if err := migrateLegacyOCIBaseline(legacyDir, storageDir); err != nil {
			t.Fatalf("migrateLegacyOCIBaseline failed: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(storageDir, baselineFileName))
		if err != nil {
			t.Fatalf("expected baseline at storage dir: %v", err)
		}
		if string(got) != legacyContent {
			t.Errorf("baseline content mismatch: got %q want %q", got, legacyContent)
		}
	})

	t.Run("existing storage baseline is not clobbered", func(t *testing.T) {
		legacyDir := t.TempDir()
		storageDir := t.TempDir()
		const newContent = "datadog-checks-base==2.0.0\n"
		if err := os.WriteFile(filepath.Join(legacyDir, baselineFileName), []byte(legacyContent), 0o644); err != nil {
			t.Fatalf("failed to seed legacy baseline: %v", err)
		}
		if err := os.WriteFile(filepath.Join(storageDir, baselineFileName), []byte(newContent), 0o644); err != nil {
			t.Fatalf("failed to seed storage baseline: %v", err)
		}

		if err := migrateLegacyOCIBaseline(legacyDir, storageDir); err != nil {
			t.Fatalf("migrateLegacyOCIBaseline failed: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(storageDir, baselineFileName))
		if err != nil {
			t.Fatalf("expected baseline at storage dir: %v", err)
		}
		if string(got) != newContent {
			t.Errorf("storage baseline was overwritten: got %q want %q", got, newContent)
		}
	})

	t.Run("no legacy baseline is a no-op", func(t *testing.T) {
		legacyDir := t.TempDir()
		storageDir := t.TempDir()

		if err := migrateLegacyOCIBaseline(legacyDir, storageDir); err != nil {
			t.Fatalf("migrateLegacyOCIBaseline failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(storageDir, baselineFileName)); !os.IsNotExist(err) {
			t.Errorf("expected no baseline to be created, got err=%v", err)
		}
	})
}

func TestRemoveCustomIntegrations(t *testing.T) {
	dir := t.TempDir()

	paths := []string{
		// Installed by customer outside of the agent
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/direct_url.json",
		// Installed by agent binary
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/direct_url.json",
		// Installed by agent integration command
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/direct_url.json",
		// Previous version .pyc files
		"embedded/lib/python3.8/site-packages/datadog_checks/__pycache__/__init__.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/__pycache__/errors.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/base/__pycache__/__init__.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/base/__pycache__/agent.cpython-312.pyc",
	}

	for _, relPath := range paths {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", fullPath, err)
		}
		f, err := os.Create(fullPath)
		if err != nil {
			t.Fatalf("failed to create file %s: %v", fullPath, err)
		}
		f.Close()
	}

	installedByPkgPath := filepath.Join(dir, "embedded", ".installed_by_pkg.txt")
	content := `# DO NOT REMOVE/MODIFY - used by package removal tasks
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/INSTALLER
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/METADATA
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/RECORD
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/REQUESTED
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/WHEEL
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/entry_points.txt
./embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/direct_url.json
`
	if err := os.WriteFile(installedByPkgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create .installed_by_pkg.txt: %v", err)
	}

	if err := RemoveCustomIntegrations(context.TODO(), dir); err != nil {
		t.Fatalf("RemoveCustomIntegrations failed: %v", err)
	}

	remaining := []string{
		// Installed by customer outside of the agent
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/botocore-1.38.8.dist-info/direct_url.json",
		// In the .installed_by_pkg.txt
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/datadog_activemq-5.0.0.dist-info/direct_url.json",
	}

	removed := []string{
		// Not in the .installed_by_pkg.txt
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/INSTALLER",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/METADATA",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/RECORD",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/REQUESTED",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/WHEEL",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/entry_points.txt",
		"embedded/lib/python3.12/site-packages/datadog_redisdb-5.0.0.dist-info/direct_url.json",
		"embedded/lib/python3.8/site-packages/datadog_checks/__pycache__/__init__.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/__pycache__/errors.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/base/__pycache__/__init__.cpython-312.pyc",
		"embedded/lib/python3.8/site-packages/datadog_checks/base/__pycache__/agent.cpython-312.pyc",
	}

	for _, relPath := range removed {
		fullPath := filepath.Join(dir, relPath)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, but it exists", fullPath)
		}
	}

	for _, relPath := range remaining {
		fullPath := filepath.Join(dir, relPath)
		if _, err := os.Stat(fullPath); err != nil {
			t.Errorf("expected %s to exist, but got error: %v", fullPath, err)
		}
	}
}
