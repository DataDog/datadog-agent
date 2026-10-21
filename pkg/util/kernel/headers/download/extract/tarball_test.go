// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// These tests build small header archives to check supported compression and
// layouts, rejection of unsafe entries, and preservation of files outside the
// destination when extraction fails.
package extract

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/zstd"

	"github.com/stretchr/testify/require"
)

type tarEntry struct {
	name, target, content string
	kind                  byte
	paxRecords            map[string]string
}

func tarArchive(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		hdr := &tar.Header{Name: entry.name, Linkname: entry.target, Typeflag: entry.kind, Mode: 0755}
		if entry.kind == tar.TypeXGlobalHeader {
			hdr.Mode = 0
			hdr.PAXRecords = entry.paxRecords
		}
		if entry.kind == tar.TypeReg {
			hdr.Size = int64(len(entry.content))
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if hdr.Size > 0 {
			_, err := tw.Write([]byte(entry.content))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestTarSkipsPAXGlobalHeader(t *testing.T) {
	dir := t.TempDir()
	archive := tarArchive(t,
		tarEntry{
			name:       "pax_global_header",
			kind:       tar.TypeXGlobalHeader,
			paxRecords: map[string]string{"comment": "0123456789abcdef0123456789abcdef01234567"},
		},
		tarEntry{name: "file.h", content: "header", kind: tar.TypeReg},
	)
	require.NoError(t, ExtractTarball(bytes.NewReader(archive), "headers.tgz", dir, nil))
	content, err := os.ReadFile(filepath.Join(dir, "file.h"))
	require.NoError(t, err)
	require.Equal(t, "header", string(content))
	require.NoFileExists(t, filepath.Join(dir, "pax_global_header"))
}

func TestTarPreservesHeaderLayout(t *testing.T) {
	dir := t.TempDir()
	archive := tarArchive(t,
		tarEntry{name: "./", kind: tar.TypeDir},
		tarEntry{name: "/usr/src/headers", kind: tar.TypeDir},
		tarEntry{name: "usr/src/headers", kind: tar.TypeDir},
		tarEntry{name: "usr/src/headers/types.h", content: "headers", kind: tar.TypeReg},
		tarEntry{name: "lib/modules/test/build", target: "/usr/src/headers", kind: tar.TypeSymlink},
		tarEntry{name: "lib/modules/test/source", target: "../../../usr/src/headers", kind: tar.TypeSymlink},
		tarEntry{name: "usr/src/headers/future.h", target: "later.h", kind: tar.TypeSymlink},
		tarEntry{name: "usr/src/headers/later.h", content: "later", kind: tar.TypeReg},
	)
	require.NoError(t, ExtractTarball(bytes.NewReader(archive), "headers.tgz", dir, nil))
	for _, name := range []string{"lib/modules/test/build/types.h", "lib/modules/test/source/types.h"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, "headers", string(content))
	}
	link, err := os.Readlink(filepath.Join(dir, "lib/modules/test/build"))
	require.NoError(t, err)
	require.Equal(t, "../../../usr/src/headers", link)
	content, err := os.ReadFile(filepath.Join(dir, "usr/src/headers/future.h"))
	require.NoError(t, err)
	require.Equal(t, "later", string(content))
	// Re-extraction must tolerate duplicate directories and identical links.
	require.NoError(t, ExtractTarball(bytes.NewReader(archive), "headers.tgz", dir, nil))
}

func TestTarRejectsEscapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []tarEntry
	}{
		{"parent path", []tarEntry{{name: "../sentinel", content: "changed", kind: tar.TypeReg}}},
		{"escaping symlink", []tarEntry{{name: "escape", target: "../", kind: tar.TypeSymlink}, {name: "escape/sentinel", content: "changed", kind: tar.TypeReg}}},
		{"symlink chain", []tarEntry{{name: "a", target: "b", kind: tar.TypeSymlink}, {name: "b", target: "../", kind: tar.TypeSymlink}}},
		{"parent after symlink", []tarEntry{{name: "a", kind: tar.TypeDir}, {name: "a/b", target: "..", kind: tar.TypeSymlink}, {name: "a/c", target: "b/../sentinel", kind: tar.TypeSymlink}}},
		{"late symlink", []tarEntry{{name: "a", kind: tar.TypeDir}, {name: "a/c", target: "b/../sentinel", kind: tar.TypeSymlink}, {name: "a/b", target: "..", kind: tar.TypeSymlink}}},
		{"cycle", []tarEntry{{name: "a", target: "b", kind: tar.TypeSymlink}, {name: "b", target: "a", kind: tar.TypeSymlink}}},
		{"hardlink", []tarEntry{{name: "file", content: "ok", kind: tar.TypeReg}, {name: "link", target: "file", kind: tar.TypeLink}}},
		{"symlinked parent", []tarEntry{{name: "real", kind: tar.TypeDir}, {name: "link", target: "real", kind: tar.TypeSymlink}, {name: "link/file", content: "changed", kind: tar.TypeReg}}},
		{"symlinked directory entry", []tarEntry{{name: "real", kind: tar.TypeDir}, {name: "link", target: "real", kind: tar.TypeSymlink}, {name: "link", kind: tar.TypeDir}}},
		{"overwrite directory", []tarEntry{{name: "dir", kind: tar.TypeDir}, {name: "dir", content: "changed", kind: tar.TypeReg}}},
		{"absolute parent target", []tarEntry{{name: "link", target: "/usr/../sentinel", kind: tar.TypeSymlink}}},
		{"hardlink escape", []tarEntry{{name: "link", target: "../sentinel", kind: tar.TypeLink}}},
		{"hardlink to symlink", []tarEntry{{name: "file", content: "ok", kind: tar.TypeReg}, {name: "sym", target: "file", kind: tar.TypeSymlink}, {name: "hard", target: "sym", kind: tar.TypeLink}}},
		{"unresolved hardlink", []tarEntry{{name: "link", target: "absent", kind: tar.TypeLink}}},
		{"fifo", []tarEntry{{name: "fifo", kind: tar.TypeFifo}}},
		{"overwrite symlink", []tarEntry{{name: "target", content: "ok", kind: tar.TypeReg}, {name: "link", target: "target", kind: tar.TypeSymlink}, {name: "link", content: "changed", kind: tar.TypeReg}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "extract")
			require.NoError(t, os.Mkdir(dir, 0755))
			sentinel := filepath.Join(parent, "sentinel")
			require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0600))
			require.Error(t, ExtractTarball(bytes.NewReader(tarArchive(t, tc.entries...)), "headers.tgz", dir, nil))
			content, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "untouched", string(content))
		})
	}
}

func TestTarRejectsExistingAncestorSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escape")))
	archive := tarArchive(t, tarEntry{name: "escape/sentinel", content: "changed", kind: tar.TypeReg})
	require.Error(t, ExtractTarball(bytes.NewReader(archive), "headers.tgz", dir, nil))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "untouched", string(content))
}

//go:embed testdata/header.tar.*
var compressedHeaders embed.FS

func TestTarCompressionFormats(t *testing.T) {
	for _, extension := range []string{"xz", "bz2", "gz", "zst"} {
		t.Run(extension, func(t *testing.T) {
			var data []byte
			var err error
			switch extension {
			case "gz", "zst":
				data = tarArchive(t, tarEntry{name: "file.h", content: "header", kind: tar.TypeReg})
				if extension == "zst" {
					reader, e := gzip.NewReader(bytes.NewReader(data))
					require.NoError(t, e)
					raw, e := io.ReadAll(reader)
					require.NoError(t, e)
					require.NoError(t, reader.Close())
					data, err = zstd.Compress(nil, raw)
				}
			default:
				data, err = compressedHeaders.ReadFile("testdata/header.tar." + extension)
			}
			require.NoError(t, err)
			dir := t.TempDir()
			require.NoError(t, ExtractTarball(bytes.NewReader(data), "header.tar."+extension, dir, nil))
			content, err := os.ReadFile(filepath.Join(dir, "file.h"))
			require.NoError(t, err)
			require.Equal(t, "header", string(content))
		})
	}
}

func TestTarRejectsDamagedCompression(t *testing.T) {
	archive := tarArchive(t, tarEntry{name: "file.h", content: "header", kind: tar.TypeReg})
	for _, damaged := range [][]byte{archive[:len(archive)-4], append(append([]byte(nil), archive[:len(archive)-8]...), make([]byte, 8)...)} {
		require.Error(t, ExtractTarball(bytes.NewReader(damaged), "headers.tgz", t.TempDir(), nil))
	}
}
