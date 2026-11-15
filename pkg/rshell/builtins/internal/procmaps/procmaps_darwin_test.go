// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package procmaps

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadImplDarwin_HappyPath(t *testing.T) {
	name, mappings, err := readImpl(context.Background(), "", os.Getpid(), false)
	assert.NoError(t, err)
	assert.NotEmpty(t, name)
	assert.NotEmpty(t, mappings)
	for _, m := range mappings {
		assert.Less(t, m.Start, m.End)
		assert.Len(t, m.Perms, 5)
	}
}

func TestReadImplDarwin_ExtendedNotSupported(t *testing.T) {
	_, _, err := readImpl(context.Background(), "", os.Getpid(), true)
	assert.ErrorIs(t, err, ErrExtendedNotSupported)
}

func TestReadImplDarwin_NoSuchProcess(t *testing.T) {
	_, _, err := readImpl(context.Background(), "", 2147483647, false)
	assert.ErrorIs(t, err, ErrNoSuchProcess)
}

func TestReadImplDarwin_InvalidPID(t *testing.T) {
	_, _, err := readImpl(context.Background(), "", 0, false)
	assert.ErrorIs(t, err, ErrNoSuchProcess)

	_, _, err = readImpl(context.Background(), "", -1, false)
	assert.ErrorIs(t, err, ErrNoSuchProcess)
}

// PID 1 (launchd) is root-owned; proc_pidinfo rejects the region walk with
// EPERM for a non-root caller. This must surface as an error rather than a
// silently empty mapping list.
func TestReadImplDarwin_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: PID 1's regions are readable, permission-denied path not exercised")
	}
	_, mappings, err := readImpl(context.Background(), "", 1, false)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrNoSuchProcess))
	assert.Empty(t, mappings)
}

func TestReadImplDarwin_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, mappings, err := readImpl(ctx, "", os.Getpid(), false)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, mappings)
}

func TestDarwinProtToMode(t *testing.T) {
	cases := map[uint32]string{
		0x0: "-----",
		0x1: "r----",
		0x2: "-w---",
		0x4: "--x--",
		0x5: "r-x--",
		0x7: "rwx--",
	}
	for prot, want := range cases {
		assert.Equal(t, want, darwinProtToMode(prot), "prot %#x", prot)
	}
}

func TestDarwinCommName(t *testing.T) {
	assert.Equal(t, "example", darwinCommName([]byte("example\x00\x00\x00")))
	assert.Equal(t, "", darwinCommName([]byte{0, 0, 0}))
}
