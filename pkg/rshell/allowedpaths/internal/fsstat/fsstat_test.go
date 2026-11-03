// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fsstat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRequiresDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close() //nolint:errcheck

	if _, err := Read(root, "file", true); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("Read(file, requireDirectory=true) = %v, want ErrNotDirectory", err)
	}
	if _, err := Read(root, "subdir", true); err != nil {
		t.Fatalf("Read(subdir, requireDirectory=true) = %v", err)
	}
}
