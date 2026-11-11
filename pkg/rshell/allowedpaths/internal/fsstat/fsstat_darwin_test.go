// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package fsstat

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadDarwin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close() //nolint:errcheck

	info, err := Read(root, "file", false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IDAvailable || info.NameMaxAvailable || !info.TypeIDAvailable || !info.FilesAvailable {
		t.Fatalf("unexpected field availability: %+v", info)
	}
	if info.TypeName == "" {
		t.Fatal("filesystem type name is empty")
	}
	if info.IOBlockSize == 0 || info.FundamentalBlockSize == 0 || info.Blocks == 0 {
		t.Fatalf("unexpected filesystem sizes: %+v", info)
	}
	if info.IOBlockSize != info.FundamentalBlockSize {
		t.Fatalf("Darwin block sizes differ: IO=%d fundamental=%d", info.IOBlockSize, info.FundamentalBlockSize)
	}
}

func TestReadDarwinRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close() //nolint:errcheck

	_, err = Read(root, "link", false)
	if !errors.Is(err, ErrPathChanged) {
		t.Fatalf("Read symlink error = %v, want ErrPathChanged", err)
	}
}

func TestReadDarwinMetadataOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unreadable"), nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "unreadable-dir"), 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "search-only-dir"), 0o100); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close() //nolint:errcheck

	for _, path := range []string{"unreadable", "unreadable-dir", "search-only-dir", "fifo"} {
		if _, err := Read(root, path, false); err != nil {
			t.Errorf("Read(%q) = %v", path, err)
		}
	}
}
