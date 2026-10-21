// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sassoftware/go-rpmutils/cpio"
	"github.com/stretchr/testify/require"
)

type cpioEntry struct {
	name, content              string
	mode, inode, links, device uint32
}

func cpioArchive(entries ...cpioEntry) []byte {
	var out bytes.Buffer
	entries = append(append([]cpioEntry(nil), entries...), cpioEntry{name: cpio.TRAILER})
	for _, destination := range entries {
		out.WriteString("070701")
		for _, field := range []uint32{destination.inode, destination.mode, 0, 0, destination.links, 0, uint32(len(destination.content)), destination.device, 0, 0, 0, uint32(len(destination.name) + 1), 0} {
			fmt.Fprintf(&out, "%08x", field)
		}
		out.WriteString(destination.name)
		out.WriteByte(0)
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
		out.WriteString(destination.content)
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
	}
	return out.Bytes()
}

func TestCPIOHeaderLayout(t *testing.T) {
	dir := t.TempDir()
	destination, err := openExtractionDirectory(dir)
	require.NoError(t, err)
	defer destination.root.Close()
	data := cpioArchive(
		cpioEntry{name: "./usr/src/headers", mode: cpio.S_ISDIR | 0755},
		cpioEntry{name: "./usr/src/headers", mode: cpio.S_ISDIR | 0755},
		cpioEntry{name: "usr/src/headers/forward.h", mode: cpio.S_ISREG | 0644, inode: 1, links: 3},
		cpioEntry{name: "usr/src/headers/types.h", content: "header", mode: cpio.S_ISREG | 0644, inode: 1, links: 3},
		cpioEntry{name: "usr/src/headers/backward.h", mode: cpio.S_ISREG | 0644, inode: 1, links: 3},
		cpioEntry{name: "usr/src/headers/empty1", mode: cpio.S_ISREG | 0644, inode: 2, links: 2},
		cpioEntry{name: "usr/src/headers/empty2", mode: cpio.S_ISREG | 0644, inode: 2, links: 2},
		cpioEntry{name: "usr/src/headers/device2", content: "different", mode: cpio.S_ISREG | 0644, inode: 1, links: 2, device: 2},
		cpioEntry{name: "usr/src/headers/future.h", content: "later.h", mode: cpio.S_ISLNK},
		cpioEntry{name: "usr/src/headers/later.h", content: "later", mode: cpio.S_ISREG},
		cpioEntry{name: "lib/modules/test/build", content: "/usr/src/headers", mode: cpio.S_ISLNK | 0777},
		cpioEntry{name: "lib/modules/test/source", content: "../../../usr/src/headers", mode: cpio.S_ISLNK | 0777},
	)
	require.NoError(t, extractRPMPayload(io.NopCloser(bytes.NewReader(data)), destination))
	for _, name := range []string{"lib/modules/test/build/types.h", "lib/modules/test/source/forward.h", "usr/src/headers/backward.h"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, "header", string(content))
	}
	one, err := os.Stat(filepath.Join(dir, "usr/src/headers/empty1"))
	require.NoError(t, err)
	two, err := os.Stat(filepath.Join(dir, "usr/src/headers/empty2"))
	require.NoError(t, err)
	require.True(t, os.SameFile(one, two))
	require.Zero(t, one.Size())
	content, err := os.ReadFile(filepath.Join(dir, "usr/src/headers/device2"))
	require.NoError(t, err)
	require.Equal(t, "different", string(content))
}

func TestCPIORejectsEscapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []cpioEntry
	}{
		{"parent path", []cpioEntry{{name: "../sentinel", content: "changed", mode: cpio.S_ISREG}}},
		{"escaping symlink", []cpioEntry{{name: "link", content: "../", mode: cpio.S_ISLNK}, {name: "link/sentinel", content: "changed", mode: cpio.S_ISREG}}},
		{"symlinked parent", []cpioEntry{{name: "real", mode: cpio.S_ISDIR}, {name: "link", content: "real", mode: cpio.S_ISLNK}, {name: "link/file", content: "changed", mode: cpio.S_ISREG}}},
		{"hardlink symlinked parent", []cpioEntry{{name: "real", mode: cpio.S_ISDIR}, {name: "link", content: "real", mode: cpio.S_ISLNK}, {name: "link/file", mode: cpio.S_ISREG, links: 2, inode: 1}, {name: "file", content: "changed", mode: cpio.S_ISREG, links: 2, inode: 1}}},
		{"ambiguous target", []cpioEntry{{name: "link", content: "other/../file", mode: cpio.S_ISLNK}}},
		{"cycle", []cpioEntry{{name: "a", content: "b", mode: cpio.S_ISLNK}, {name: "b", content: "a", mode: cpio.S_ISLNK}}},
		{"overwrite directory", []cpioEntry{{name: "dir", mode: cpio.S_ISDIR}, {name: "dir", content: "changed", mode: cpio.S_ISREG}}},
		{"overwrite symlink", []cpioEntry{{name: "link", content: "target", mode: cpio.S_ISLNK}, {name: "link", content: "changed", mode: cpio.S_ISREG}}},
		{"hardlink escape", []cpioEntry{{name: "../sentinel", mode: cpio.S_ISREG, links: 2, inode: 1}, {name: "file", content: "changed", mode: cpio.S_ISREG, links: 2, inode: 1}}},
		{"device", []cpioEntry{{name: "dev", mode: cpio.S_ISCHR}}},
		{"fifo", []cpioEntry{{name: "fifo", mode: cpio.S_ISFIFO}}},
		{"socket", []cpioEntry{{name: "socket", mode: cpio.S_ISSOCK}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "extract")
			require.NoError(t, os.Mkdir(dir, 0755))
			sentinel := filepath.Join(parent, "sentinel")
			require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0600))
			destination, err := openExtractionDirectory(dir)
			require.NoError(t, err)
			defer destination.root.Close()
			require.Error(t, extractRPMPayload(io.NopCloser(bytes.NewReader(cpioArchive(tc.entries...))), destination))
			content, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "untouched", string(content))
		})
	}
}

type trackedPayload struct {
	io.Reader
	closed bool
}

func (p *trackedPayload) Close() error { p.closed = true; return nil }

func TestCPIOTruncationClosesDecoder(t *testing.T) {
	data := cpioArchive(cpioEntry{name: "file", content: "content", mode: cpio.S_ISREG})
	for _, end := range []int{0, 6, 120, len(data) - 10, len(data)} {
		t.Run(strconv.Itoa(end), func(t *testing.T) {
			destination, err := openExtractionDirectory(t.TempDir())
			require.NoError(t, err)
			defer destination.root.Close()
			payload := &trackedPayload{Reader: bytes.NewReader(data[:end])}
			err = extractRPMPayload(payload, destination)
			if end == len(data) {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.True(t, payload.closed)
		})
	}
}

func TestRPMCompressionFormats(t *testing.T) {
	entries, err := filepath.Glob("testdata/payload-test-*.rpm")
	require.NoError(t, err)
	require.Len(t, entries, 6)
	for _, entry := range entries {
		t.Run(filepath.Base(entry), func(t *testing.T) {
			data, err := os.ReadFile(entry)
			require.NoError(t, err)
			dir := t.TempDir()
			require.NoError(t, ExtractRPMPackage(bytes.NewReader(data), dir, "test", nil))
			content, err := os.ReadFile(filepath.Join(dir, "usr/share/payload-test.txt"))
			require.NoError(t, err)
			require.Equal(t, "Some data\n", string(content))
		})
	}
}

func TestCPIOChecksumErrorClosesDecoder(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(cpioArchive(cpioEntry{name: "file.h", content: "header", mode: cpio.S_ISREG}))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	data := compressed.Bytes()
	data[len(data)-8] ^= 0xff
	decoder, err := openCompressed(bytes.NewReader(data), compressionGzip)
	require.NoError(t, err)
	payload := &trackedPayload{Reader: decoder}
	defer decoder.Close()
	destination, err := openExtractionDirectory(t.TempDir())
	require.NoError(t, err)
	defer destination.root.Close()
	require.ErrorIs(t, extractRPMPayload(payload, destination), gzip.ErrChecksum)
	require.True(t, payload.closed)
}
