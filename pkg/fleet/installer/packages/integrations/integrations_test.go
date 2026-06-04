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

func TestMigrateLegacyOCIFile(t *testing.T) {
	const legacyContent = "datadog-ping==1.0.2\n"
	const existingContent = "datadog-postgres==7.0.0\n"

	tests := []struct {
		name           string
		legacyContent  string // "" means no legacy file
		storageContent string // "" means no pre-existing storage file
		want           string // "" means the storage file should not exist
	}{
		{"migrated when storage absent", legacyContent, "", legacyContent},
		{"existing storage file not clobbered", legacyContent, existingContent, existingContent},
		{"no legacy file is a no-op", "", "", ""},
	}

	for _, fileName := range []string{baselineFileName, diffFileName} {
		for _, tt := range tests {
			t.Run(fileName+"/"+tt.name, func(t *testing.T) {
				legacyDir := t.TempDir()
				storageDir := filepath.Join(t.TempDir(), "run") // not pre-created; exercises MkdirAll
				if tt.legacyContent != "" {
					if err := os.WriteFile(filepath.Join(legacyDir, fileName), []byte(tt.legacyContent), 0o644); err != nil {
						t.Fatalf("failed to seed legacy file: %v", err)
					}
				}
				if tt.storageContent != "" {
					if err := os.MkdirAll(storageDir, 0o755); err != nil {
						t.Fatalf("failed to create storage dir: %v", err)
					}
					if err := os.WriteFile(filepath.Join(storageDir, fileName), []byte(tt.storageContent), 0o644); err != nil {
						t.Fatalf("failed to seed storage file: %v", err)
					}
				}

				if err := migrateLegacyOCIFile(legacyDir, storageDir, fileName); err != nil {
					t.Fatalf("migrateLegacyOCIFile failed: %v", err)
				}

				got, err := os.ReadFile(filepath.Join(storageDir, fileName))
				if tt.want == "" {
					if !os.IsNotExist(err) {
						t.Errorf("expected no storage file, got %q (err %v)", got, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("expected file at storage dir: %v", err)
				}
				if string(got) != tt.want {
					t.Errorf("content mismatch: got %q want %q", got, tt.want)
				}
			})
		}
	}
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
