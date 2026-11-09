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
	"time"
)

func TestMigrateLegacyOCIFile(t *testing.T) {
	t.Parallel()

	const legacyContent = "datadog-ping==1.0.2\n"
	const existingContent = "datadog-postgres==7.0.0\n"

	tests := []struct {
		name           string
		legacyContent  string // "" means no legacy file
		storageContent string // "" means no pre-existing storage file
		legacyNewer    bool   // when both files exist, stamp legacy newer than storage
		equalMtime     bool   // when both files exist, stamp identical mtimes (tie-break)
		want           string // "" means the storage file should not exist
	}{
		{"migrated when storage absent", legacyContent, "", false, false, legacyContent},
		{"stale storage refreshed from newer legacy", legacyContent, existingContent, true, false, legacyContent},
		{"newer storage not clobbered by older legacy", legacyContent, existingContent, false, false, existingContent},
		{"equal mtime refreshes when content differs", legacyContent, existingContent, false, true, legacyContent},
		{"equal mtime kept when content identical", legacyContent, legacyContent, false, true, legacyContent},
		{"no legacy file is a no-op", "", "", false, false, ""},
	}

	for _, fileName := range []string{baselineFileName, diffFileName} {
		for _, tt := range tests {
			t.Run(fileName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
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
				if tt.legacyContent != "" && tt.storageContent != "" {
					// Stamp deterministic mtimes so the refresh decision does not depend
					// on filesystem timestamp granularity between the two writes above.
					older, newer := time.Now().Add(-time.Hour), time.Now()
					legacyTime, storageTime := older, newer
					switch {
					case tt.equalMtime:
						legacyTime, storageTime = newer, newer
					case tt.legacyNewer:
						legacyTime, storageTime = newer, older
					}
					if err := os.Chtimes(filepath.Join(legacyDir, fileName), legacyTime, legacyTime); err != nil {
						t.Fatalf("failed to set legacy mtime: %v", err)
					}
					if err := os.Chtimes(filepath.Join(storageDir, fileName), storageTime, storageTime); err != nil {
						t.Fatalf("failed to set storage mtime: %v", err)
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

func TestCopyOCIFile(t *testing.T) {
	t.Parallel()

	const srcContent = "datadog-ping==1.0.2\n"
	const dstContent = "datadog-postgres==7.0.0\n"

	tests := []struct {
		name       string
		srcContent string // "" means no source file
		dstContent string // "" means no pre-existing destination file
		want       string // "" means the destination file should not exist
	}{
		{"copied when destination absent", srcContent, "", srcContent},
		{"overwrites existing destination", srcContent, dstContent, srcContent},
		{"no source file is a no-op", "", dstContent, dstContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDir := t.TempDir()
			dstDir := filepath.Join(t.TempDir(), "tmp") // not pre-created; exercises MkdirAll
			if tt.srcContent != "" {
				if err := os.WriteFile(filepath.Join(srcDir, diffFileName), []byte(tt.srcContent), 0o644); err != nil {
					t.Fatalf("failed to seed source file: %v", err)
				}
			}
			if tt.dstContent != "" {
				if err := os.MkdirAll(dstDir, 0o755); err != nil {
					t.Fatalf("failed to create destination dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dstDir, diffFileName), []byte(tt.dstContent), 0o644); err != nil {
					t.Fatalf("failed to seed destination file: %v", err)
				}
			}

			if err := copyOCIFile(srcDir, dstDir, diffFileName); err != nil {
				t.Fatalf("copyOCIFile failed: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(dstDir, diffFileName))
			if tt.want == "" {
				if !os.IsNotExist(err) {
					t.Errorf("expected no destination file, got %q (err %v)", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected file at destination dir: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("content mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestMirrorOCIFile(t *testing.T) {
	t.Parallel()

	const srcContent = "datadog-ping==1.0.2\n"
	const dstContent = "datadog-postgres==7.0.0\n"

	tests := []struct {
		name         string
		srcContent   string // "" means no source file
		dstDirExists bool
		dstContent   string // "" means no pre-existing destination file
		wantContent  string // "" means the destination file should not exist
		wantDir      bool   // whether dstDir should exist afterwards
	}{
		{"copied when dest dir exists", srcContent, true, "", srcContent, true},
		{"skipped when dest dir absent", srcContent, false, "", "", false},
		{"no source file is a no-op", "", true, dstContent, dstContent, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDir := t.TempDir()
			dstDir := filepath.Join(t.TempDir(), "tmp") // not pre-created unless the case asks for it
			if tt.srcContent != "" {
				if err := os.WriteFile(filepath.Join(srcDir, diffFileName), []byte(tt.srcContent), 0o644); err != nil {
					t.Fatalf("failed to seed source file: %v", err)
				}
			}
			if tt.dstDirExists {
				if err := os.MkdirAll(dstDir, 0o755); err != nil {
					t.Fatalf("failed to create destination dir: %v", err)
				}
			}
			if tt.dstContent != "" {
				if err := os.WriteFile(filepath.Join(dstDir, diffFileName), []byte(tt.dstContent), 0o644); err != nil {
					t.Fatalf("failed to seed destination file: %v", err)
				}
			}

			if err := mirrorOCIFile(srcDir, dstDir, diffFileName); err != nil {
				t.Fatalf("mirrorOCIFile failed: %v", err)
			}

			if _, err := os.Stat(dstDir); tt.wantDir != (err == nil) {
				t.Errorf("dstDir existence: got err %v, wantDir %v", err, tt.wantDir)
			}

			got, err := os.ReadFile(filepath.Join(dstDir, diffFileName))
			if tt.wantContent == "" {
				if !os.IsNotExist(err) {
					t.Errorf("expected no destination file, got %q (err %v)", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected file at destination dir: %v", err)
			}
			if string(got) != tt.wantContent {
				t.Errorf("content mismatch: got %q want %q", got, tt.wantContent)
			}
		})
	}
}

func TestMirrorOCIFileStatError(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, diffFileName), []byte("datadog-ping==1.0.2\n"), 0o644); err != nil {
		t.Fatalf("failed to seed source file: %v", err)
	}
	// A regular file where a directory component is expected makes os.Stat(dstDir)
	// fail with ENOTDIR rather than os.IsNotExist, exercising the error branch.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("failed to create stat blocker: %v", err)
	}
	dstDir := filepath.Join(blocker, "tmp")

	if err := mirrorOCIFile(srcDir, dstDir, diffFileName); err == nil {
		t.Fatal("expected an error from a non-IsNotExist stat, got nil")
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

func TestRemoveCompiledFiles(t *testing.T) {
	dir := t.TempDir()

	removed := []string{
		"embedded/lib/python3.12/site-packages/dateutil/__pycache__/parser.cpython-312.pyc",
		"embedded/lib/python3.12/site-packages/dateutil/__pycache__/tz.cpython-312.opt-1.pyc",
		"bin/agent/dist/checks/__pycache__/foo.cpython-312.pyc",
		"python-scripts/__pycache__/pre.cpython-312.pyc",
	}
	remaining := []string{
		"embedded/lib/python3.12/site-packages/dateutil/parser.py",
		"embedded/lib/python3.12/site-packages/legacy/module.py",
	}

	for _, relPath := range append(removed, remaining...) {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", fullPath, err)
		}
	}

	if err := RemoveCompiledFiles(dir); err != nil {
		t.Fatalf("RemoveCompiledFiles failed: %v", err)
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
	if _, err := os.Stat(filepath.Join(dir, "embedded/lib/python3.12/site-packages/dateutil/__pycache__")); !os.IsNotExist(err) {
		t.Errorf("expected __pycache__ directory to be removed")
	}
}
