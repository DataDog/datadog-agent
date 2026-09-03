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
		skip  int
		count int
		want  []byte
	}{
		{name: "aligned start", skip: 0, count: 100, want: content[:100]},
		{name: "unaligned start", skip: 137, count: 100, want: content[137:237]},
		{name: "spans block boundary", skip: directIOAlignment - 1, count: 3, want: content[directIOAlignment-1 : directIOAlignment+2]},
		{name: "deep skip stays bounded", skip: window + 500, count: 64, want: content[window+500 : window+564]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := openTestFile(t, path)
			got, err := readDirectFingerprintRangeFromFile(file, tt.skip, tt.count, directIOAlignment, directIOAlignment)
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
			got, err := readDirectFingerprintRangeFromFile(file, 0, size+16, directIOAlignment, directIOAlignment)
			require.NoError(t, err)
			require.Equal(t, content, got)
		})
	}
}

func TestReadDirectFingerprintRangeRejectsInvalidArgs(t *testing.T) {
	path, _ := writeTestFile(t, "args.log", 16)
	_, err := readDirectFingerprintRangeFromFile(openTestFile(t, path), -1, 4, directIOAlignment, directIOAlignment)
	require.ErrorIs(t, err, os.ErrInvalid)
}
