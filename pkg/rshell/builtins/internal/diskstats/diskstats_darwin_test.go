// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package diskstats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestList_Darwin_HappyPath(t *testing.T) {
	mounts, err := List(context.Background(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, mounts, "macOS should always have at least one mount")

	var foundRoot bool
	for _, m := range mounts {
		if m.MountPoint == "/" {
			foundRoot = true
			assert.NotEmpty(t, m.FSType, "root FS type must be set")
			assert.NotZero(t, m.Total, "root must report non-zero total")
			assert.NotZero(t, m.BlockSize, "root must report non-zero block size")
			break
		}
	}
	assert.True(t, foundRoot, "macOS listing should include root mount")
}

func TestList_Darwin_UsedNeverNegative(t *testing.T) {
	// Used is computed via saturated subtraction; verify no mount
	// produces a wrap-around (a sign the implementation is buggy).
	mounts, err := List(context.Background(), nil)
	require.NoError(t, err)
	for _, m := range mounts {
		// Used must be ≤ Total (modulo root reservation), never the
		// uint64 wrap value. A wrap would show ~18 EB, well above
		// any realistic FS size.
		assert.Less(t, m.Used, uint64(1)<<60, "mount %q used wrapped", m.MountPoint)
	}
}

func TestList_Darwin_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

// byteSliceToString was a local NUL-terminator helper; it has been
// replaced by golang.org/x/sys/unix.ByteSliceToString. This test is
// retained as a sanity check on the underlying behaviour.
func TestUnixByteSliceToString(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", []byte{}, ""},
		{"all-zero", []byte{0, 0, 0, 0}, ""},
		{"trailing-zero", []byte{'h', 'i', 0, 0, 0}, "hi"},
		{"no-zero", []byte("hello"), "hello"},
		{"zero-at-zero", []byte{0, 'x', 'y'}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, unix.ByteSliceToString(c.in))
		})
	}
}
