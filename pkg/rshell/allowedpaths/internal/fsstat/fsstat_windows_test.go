// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package fsstat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadWindows(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	info, err := Read(root, "file", false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IDAvailable {
		t.Error("volume ID is unavailable")
	}
	if !info.NameMaxAvailable || info.NameMax == 0 {
		t.Errorf("invalid maximum component length: available=%v value=%d", info.NameMaxAvailable, info.NameMax)
	}
	if info.TypeName == "" {
		t.Error("filesystem type name is empty")
	}
	if info.IOBlockSize == 0 || info.FundamentalBlockSize == 0 {
		t.Errorf("invalid block sizes: IO=%d fundamental=%d", info.IOBlockSize, info.FundamentalBlockSize)
	}
	if info.Blocks == 0 {
		t.Error("total block count is zero")
	}
	if info.FilesAvailable {
		t.Error("Windows must not report POSIX inode counts as available")
	}
}

func TestReadWindowsRejectsNonLocalPath(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	_, err = Read(root, `..\outside`, false)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Read returned %v, want os.ErrInvalid", err)
	}
}

func TestReadWindowsReportsRacedReparsePoint(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("creating a symlink requires unavailable Windows privileges: %v", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	_, err = Read(root, "link", false)
	if !errors.Is(err, ErrPathChanged) {
		t.Fatalf("Read returned %v, want ErrPathChanged", err)
	}
}
