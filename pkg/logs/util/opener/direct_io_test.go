// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opener

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, name string, size int) (string, []byte) {
	t.Helper()
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, content, 0600))
	return path, content
}

func openTestFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestReadDirectFingerprintRangeReturnsLogicalRange(t *testing.T) {
	const window = directIOAlignment * 4
	path, content := writeTestFile(t, "ranges.log", window*2+137)

	tests := []struct {
		name  string
		count int
		want  []byte
	}{
		{name: "aligned count", count: directIOAlignment, want: content[:directIOAlignment]},
		{name: "unaligned count", count: 100, want: content[:100]},
		{name: "spans block boundary", count: directIOAlignment + 2, want: content[:directIOAlignment+2]},
		{name: "spans several blocks", count: window + 500, want: content[:window+500]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := openTestFile(t, path)
			got, err := readDirectFingerprintRangeFromFile(file, tt.count, directIOAlignment, directIOAlignment)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReadDirectFingerprintRangeStopsAtEOF(t *testing.T) {
	for _, size := range []int{0, 1, 511, 512, 777, 2047, 4096, 4097, 8193} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			path, content := writeTestFile(t, "short.log", size)
			file := openTestFile(t, path)
			got, err := readDirectFingerprintRangeFromFile(file, size+16, directIOAlignment, directIOAlignment)
			require.NoError(t, err)
			require.Equal(t, content, got)
		})
	}
}

func TestReadDirectFingerprintRangeRejectsInvalidArgs(t *testing.T) {
	path, _ := writeTestFile(t, "args.log", 16)
	_, err := readDirectFingerprintRangeFromFile(openTestFile(t, path), -1, directIOAlignment, directIOAlignment)
	require.ErrorIs(t, err, os.ErrInvalid)
}
