// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// requireDirectIOTestFile writes content to a file on a filesystem that accepts
// O_DIRECT and returns its path.
//
// These are the only tests that drive a real direct-I/O descriptor; everything
// else in the package runs against ordinary buffered files and so never
// exercises the kernel's alignment constraints. t.TempDir() follows TMPDIR and
// frequently lands on tmpfs, which rejects O_DIRECT outright, so skipping on the
// first failure would leave those constraints unverified while CI stayed green.
// /var/tmp is disk backed on a normal Linux host and is tried before giving up.
func requireDirectIOTestFile(t *testing.T, name string, content []byte) string {
	t.Helper()

	var refusals []string
	for _, dir := range directIOCandidateDirs(t) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, content, 0600))

		file, err := openDirect(path)
		if err == nil {
			require.NoError(t, file.Close())
			return path
		}
		// Only a genuine "this filesystem cannot do it" is worth trying the next
		// candidate for; anything else is a real failure.
		require.True(t, IsOpenFlagsUnsupportedError(err), "unexpected error opening %s with O_DIRECT: %v", path, err)
		refusals = append(refusals, fmt.Sprintf("%s (%v)", dir, err))
	}

	// Set DD_REQUIRE_O_DIRECT_TESTS to turn the skip into a failure, so an
	// environment that is supposed to cover direct I/O cannot quietly stop.
	message := "no candidate filesystem supports O_DIRECT: " + strings.Join(refusals, ", ")
	if os.Getenv("DD_REQUIRE_O_DIRECT_TESTS") != "" {
		t.Fatal(message)
	}
	t.Skip(message)
	return ""
}

func directIOCandidateDirs(t *testing.T) []string {
	t.Helper()
	dirs := []string{t.TempDir()}
	if dir, err := os.MkdirTemp("/var/tmp", "dd-directio"); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		dirs = append(dirs, dir)
	}
	return dirs
}

// openDirectReader opens an already-proven-suitable path with O_DIRECT.
func openDirectReader(t *testing.T, path string) io.ReadSeekCloser {
	t.Helper()
	reader, err := NewFileOpener().OpenReaderWithFlags(path, []types.FileOpenFlag{types.FileOpenFlagDirect})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

// TestOpenReaderWithDirect exercises a real O_DIRECT descriptor end to end. The
// file size is deliberately not a multiple of the block alignment, so a read
// running to the end straddles EOF on an unaligned boundary. That is the case
// most likely to trip up the kernel's direct-I/O constraints, and it must still
// surface the exact logical bytes with no padding.
func TestOpenReaderWithDirect(t *testing.T) {
	content := make([]byte, directIOAlignment*2+211)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := requireDirectIOTestFile(t, "direct.log", content)

	tests := []struct {
		name   string
		offset int64
		length int
		want   []byte
	}{
		// The range a fingerprint configuration typically asks for: neither
		// endpoint block aligned.
		{name: "bounded range", offset: 0, length: 2061, want: content[:2061]},
		{name: "range after a skip", offset: 1037, length: 512, want: content[1037:1549]},
		{name: "range running to EOF", offset: 0, length: len(content), want: content},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := openDirectReader(t, path)

			_, err := reader.Seek(tt.offset, io.SeekStart)
			require.NoError(t, err)

			got := make([]byte, tt.length)
			read, err := io.ReadFull(reader, got)
			require.NoError(t, err)
			require.Equal(t, tt.length, read)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestOpenReaderWithDirectReadsPastEOF confirms the reader reports EOF instead
// of erroring when the caller asks for more than the file holds, which is how a
// fingerprint on a file shorter than its configured count terminates.
func TestOpenReaderWithDirectReadsPastEOF(t *testing.T) {
	content := []byte("short file\n")
	path := requireDirectIOTestFile(t, "eof.log", content)

	got, err := io.ReadAll(openDirectReader(t, path))
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestOpenReaderWithDirectReportsPermissionErrorAsUnsupported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses DAC permission checks, so O_DIRECT open would not hit EACCES")
	}
	path := filepath.Join(t.TempDir(), "noperm.log")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o000))

	_, err := NewFileOpener().OpenReaderWithFlags(path, []types.FileOpenFlag{types.FileOpenFlagDirect})
	// A file the Agent cannot open directly must be classified as unsupported
	// flags rather than as an ordinary I/O error, because that is what makes the
	// launcher report it as a configuration problem instead of a transient one.
	require.ErrorIs(t, err, ErrOpenFlagsUnsupported)
}
