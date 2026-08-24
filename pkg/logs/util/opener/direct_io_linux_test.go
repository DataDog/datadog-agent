// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestValidatedDirectIOAlignment(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  int
	}{
		{name: "zero falls back to default", value: 0, want: directIOAlignment},
		{name: "non power of two falls back", value: 3, want: directIOAlignment},
		{name: "non power of two large falls back", value: 4097, want: directIOAlignment},
		{name: "above max falls back", value: maxDirectIOAlignment * 2, want: directIOAlignment},
		{name: "valid small power of two", value: 512, want: 512},
		{name: "valid default power of two", value: directIOAlignment, want: directIOAlignment},
		{name: "valid max power of two", value: maxDirectIOAlignment, want: maxDirectIOAlignment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, validatedDirectIOAlignment(tt.value))
		})
	}
}

// TestOpenLogFileWithDirectReadsUnalignedTail exercises a real O_DIRECT
// descriptor end-to-end: it reads an entire file whose size is not a multiple
// of the block alignment, so the final aligned read straddles EOF. This is the
// case most likely to trip up the kernel's direct-I/O constraints, and it must
// still surface the exact logical bytes with a clean EOF.
func TestOpenLogFileWithDirectReadsUnalignedTail(t *testing.T) {
	content := make([]byte, directIOAlignment*2+123)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := filepath.Join(t.TempDir(), "direct-tail.log")
	require.NoError(t, os.WriteFile(path, content, 0600))

	file, err := NewFileOpener().OpenLogFileWithFlags(path, []types.FileOpenFlag{types.FileOpenFlagDirect})
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("test filesystem does not support O_DIRECT: %v", err)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	// io.ReadAll drives sequential reads until EOF, including the unaligned
	// tail block, so a mishandled EOF or leaked padding would show up here.
	got, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, content, got)
}
