// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package fsstat

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadLinux(t *testing.T) {
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
	if !info.IDAvailable || !info.NameMaxAvailable || !info.TypeIDAvailable || !info.FilesAvailable {
		t.Fatalf("unexpected unavailable field: %+v", info)
	}
	if info.TypeName == "" {
		t.Fatal("filesystem type name is empty")
	}
	if info.IOBlockSize == 0 || info.FundamentalBlockSize == 0 || info.Blocks == 0 {
		t.Fatalf("unexpected filesystem sizes: %+v", info)
	}
}

func TestReadLinuxRejectsFinalSymlink(t *testing.T) {
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

func TestLinuxTypeName(t *testing.T) {
	tests := map[uint64]string{
		0xef53:     "ext2/ext3",
		0x794c7630: "overlayfs",
		0x1021994:  "tmpfs",
	}
	for typeID, want := range tests {
		if got := linuxTypeName(typeID); got != want {
			t.Errorf("linuxTypeName(%#x) = %q, want %q", typeID, got, want)
		}
	}
	if got, want := linuxTypeName(0xdeadbeef), "UNKNOWN (0xdeadbeef)"; got != want {
		t.Fatalf("linuxTypeName(unknown) = %q, want %q", got, want)
	}
}

func TestLinuxTypeIDDoesNotSignExtend(t *testing.T) {
	raw := uint32(0xff534d42)
	signed := int32(raw)
	if got := linuxTypeID(int64(signed)); got != uint64(raw) {
		t.Fatalf("linuxTypeID(%d) = %#x, want %#x", signed, got, raw)
	}
}

func TestReadLinuxMetadataOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unreadable"), nil, 0); err != nil {
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

	for _, path := range []string{"unreadable", "fifo"} {
		if _, err := Read(root, path, false); err != nil {
			t.Errorf("Read(%q) = %v", path, err)
		}
	}
}
