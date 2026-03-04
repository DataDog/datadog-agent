// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTail_Symlink(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\n"
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.WriteFile(real, []byte(content), 0644))
	require.NoError(t, os.Symlink(real, link))

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 2 link.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_DevZeroContextCancel(t *testing.T) {
	// /dev/zero is an infinite source; context cancellation must terminate it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}))
	err := r.Run(ctx, "tail -n 5 /dev/zero")
	// Should return an error due to context cancellation.
	require.Error(t, err)
}
