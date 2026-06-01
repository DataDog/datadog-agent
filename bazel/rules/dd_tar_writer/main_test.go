// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"archive/tar"
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// — — — test helpers — — —

// writeFile creates a file in dir with the given content and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// writeExec creates an executable file and returns its path.
func writeExec(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := writeFile(t, dir, name, content)
	require.NoError(t, os.Chmod(p, 0o755))
	return p
}

// writeManifest serialises entries to JSON and writes them to path.
// Using json.Marshal ensures that file paths containing backslashes (Windows)
// are correctly escaped, unlike raw string concatenation into JSON literals.
func writeManifest(t *testing.T, path string, entries []ManifestEntry) {
	t.Helper()
	data, err := json.Marshal(entries)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// tarEntries reads all headers from a plain (uncompressed) tar file.
func tarEntries(t *testing.T, tarPath string) []*tar.Header {
	t.Helper()
	f, err := os.Open(tarPath)
	require.NoError(t, err)
	defer f.Close()
	return readTarEntries(t, tar.NewReader(f))
}

func readTarEntries(t *testing.T, tr *tar.Reader) []*tar.Header {
	t.Helper()
	var hdrs []*tar.Header
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		h := *hdr
		hdrs = append(hdrs, &h)
	}
	return hdrs
}

// tarContent reads the content of a named entry from a plain tar file.
func tarContent(t *testing.T, tarPath, entryName string) string {
	t.Helper()
	f, err := os.Open(tarPath)
	require.NoError(t, err)
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if hdr.Name == entryName {
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			return string(data)
		}
	}
	t.Fatalf("entry %q not found in %s", entryName, tarPath)
	return ""
}

// parseMd5sums reads a md5sums file into a map[path]hash.
func parseMd5sums(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// Format: "<hash>  <path>"
		parts := strings.SplitN(line, "  ", 2)
		require.Len(t, parts, 2, "invalid md5sums line: %q", line)
		result[parts[1]] = parts[0]
	}
	require.NoError(t, scanner.Err())
	return result
}

// md5OfString computes the hex MD5 of a string.
func md5OfString(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// defaultConfig returns a minimal Config for testing.
func defaultConfig(output, md5Out string) Config {
	return Config{
		Output:           output,
		Md5sumsOutput:    md5Out,
		DefaultMtime:     portableMtime,
		DefaultIDs:       [2]int{0, 0},
		DefaultNames:     [2]string{"", ""},
		ModeMap:          make(map[string]os.FileMode),
		IDsMap:           make(map[string][2]int),
		NamesMap:         make(map[string][2]string),
		CompressionLevel: -1,
	}
}

// — — — tests — — —

func TestAddFile_BasicContent(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "hello.txt", "hello world")
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "opt/agent/hello.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	// Verify tar entry.
	hdrs := tarEntries(t, out)
	require.Len(t, hdrs, 1)
	assert.Equal(t, "opt/agent/hello.txt", hdrs[0].Name)
	assert.Equal(t, tar.TypeReg, rune(hdrs[0].Typeflag))
	assert.Equal(t, int64(11), hdrs[0].Size)
	assert.Equal(t, portableMtime, hdrs[0].ModTime.Unix(), "mtime should be portable mtime")
	assert.Equal(t, int64(0o644), hdrs[0].Mode)

	// Verify content.
	assert.Equal(t, "hello world", tarContent(t, out, "opt/agent/hello.txt"))

	// Verify MD5 sidecar.
	sums := parseMd5sums(t, md5Out)
	assert.Equal(t, md5OfString("hello world"), sums["opt/agent/hello.txt"])
}

func TestAddFile_ModeOverride(t *testing.T) {
	dir := t.TempDir()
	src := writeExec(t, dir, "agent", "#!/bin/sh\n")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.ModeMap = map[string]os.FileMode{"bin/agent": 0o750}
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "bin/agent", Src: src},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	require.Len(t, hdrs, 1)
	assert.Equal(t, int64(0o750), hdrs[0].Mode)
}

func TestAddFile_ManifestModeOverride(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "config.yaml", "key: val")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	// Manifest mode takes highest precedence.
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "etc/config.yaml", Src: src, Mode: "0600"},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	assert.Equal(t, int64(0o600), hdrs[0].Mode)
}

func TestAddFile_OwnerOverride(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "f.txt", "data")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.IDsMap = map[string][2]int{"data/f.txt": {1000, 1001}}
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "data/f.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	assert.Equal(t, 1000, hdrs[0].Uid)
	assert.Equal(t, 1001, hdrs[0].Gid)
}

func TestAddSymlink_ExcludedFromMd5(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsLink, Dest: "opt/agent/current", Src: "/opt/agent/7.0.0"},
	})

	require.NoError(t, run(cfg))

	sums := parseMd5sums(t, md5Out)
	assert.Empty(t, sums, "symlinks must not appear in md5sums")
}

func TestAddDir_ExcludedFromMd5(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsDir, Dest: "opt/agent/logs"},
	})

	require.NoError(t, run(cfg))

	sums := parseMd5sums(t, md5Out)
	assert.Empty(t, sums, "directories must not appear in md5sums")
}

func TestAddEmptyFile_Md5OfEmpty(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsEmptyFile, Dest: "opt/agent/placeholder"},
	})

	require.NoError(t, run(cfg))

	sums := parseMd5sums(t, md5Out)
	require.Contains(t, sums, "opt/agent/placeholder")
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", sums["opt/agent/placeholder"], "md5 of empty string")
}

func TestDuplicateFile_FirstWins(t *testing.T) {
	dir := t.TempDir()
	src1 := writeFile(t, dir, "v1.txt", "version one")
	src2 := writeFile(t, dir, "v2.txt", "version two")
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "data/file.txt", Src: src1},
		{Type: entryIsFile, Dest: "data/file.txt", Src: src2},
	})

	require.NoError(t, run(cfg))

	// Only one entry in the tar.
	hdrs := tarEntries(t, out)
	count := 0
	for _, h := range hdrs {
		if h.Name == "data/file.txt" {
			count++
		}
	}
	assert.Equal(t, 1, count)

	// Content should be the first file.
	assert.Equal(t, "version one", tarContent(t, out, "data/file.txt"))

	// MD5 should also be for the first file.
	sums := parseMd5sums(t, md5Out)
	assert.Equal(t, md5OfString("version one"), sums["data/file.txt"])
}

// TestMergeTar_Md5sums verifies that files re-packed from a merged tar also
// appear in the md5sums sidecar (archive structure tested declaratively).
func TestMergeTar_Md5sums(t *testing.T) {
	dir := t.TempDir()

	// Create a small tar to merge.
	mergeSrc := filepath.Join(dir, "merge.tar")
	func() {
		f, err := os.Create(mergeSrc)
		require.NoError(t, err)
		defer f.Close()
		tw := tar.NewWriter(f)
		defer tw.Close()
		content := []byte("from merged tar")
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     "merged/file.txt",
			Size:     int64(len(content)),
			Mode:     0o644,
			ModTime:  time.Unix(portableMtime, 0).UTC(),
		}))
		_, err = tw.Write(content)
		require.NoError(t, err)
	}()

	src := writeFile(t, dir, "local.txt", "local content")
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.Tars = []string{mergeSrc}
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "local.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	sums := parseMd5sums(t, md5Out)
	assert.Equal(t, md5OfString("local content"), sums["local.txt"])
	assert.Equal(t, md5OfString("from merged tar"), sums["merged/file.txt"])
}

// TestAddTree_Md5sums verifies that a tree walk adds regular files to md5sums
// but excludes directories (archive structure tested declaratively).
func TestAddTree_Md5sums(t *testing.T) {
	dir := t.TempDir()

	treeRoot := filepath.Join(dir, "tree")
	writeFile(t, treeRoot, "a.txt", "aaa")
	writeFile(t, treeRoot, "sub/b.txt", "bbb")
	require.NoError(t, os.MkdirAll(filepath.Join(treeRoot, "emptydir"), 0o755))

	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsTree, Dest: "opt/agent", Src: treeRoot},
	})

	require.NoError(t, run(cfg))

	sums := parseMd5sums(t, md5Out)
	assert.Equal(t, md5OfString("aaa"), sums["opt/agent/a.txt"])
	assert.Equal(t, md5OfString("bbb"), sums["opt/agent/sub/b.txt"])
	assert.NotContains(t, sums, "opt/agent/emptydir/", "directories must not appear in md5sums")
	assert.Len(t, sums, 2)
}

func TestGnuTarFormat(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "f.txt", "x")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "f.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	require.Len(t, hdrs, 1)
	// FormatGNU or FormatUnknown (unknown after round-trip read is acceptable).
	assert.True(t, hdrs[0].Format == tar.FormatGNU || hdrs[0].Format == tar.FormatUnknown,
		"expected GNU format, got %v", hdrs[0].Format)
}

func TestExpandArgs_ResponseFile(t *testing.T) {
	dir := t.TempDir()
	paramFile := filepath.Join(dir, "args.txt")
	require.NoError(t, os.WriteFile(paramFile, []byte("--output\nout.tar\n\n--compression\ngz\n"), 0o644))

	expanded, err := expandArgs([]string{"@" + paramFile, "--manifest", "m.json"})
	require.NoError(t, err)
	assert.Equal(t, []string{"--output", "out.tar", "--compression", "gz", "--manifest", "m.json"}, expanded)
}

func TestPortableMtime(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "f.txt", "data")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.DefaultMtime = portableMtime
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "f.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	require.Len(t, hdrs, 1)
	assert.Equal(t, portableMtime, hdrs[0].ModTime.Unix(), "mtime should be portable mtime")
}

func TestMd5sumsFormat(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "f.txt", "hello")
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "opt/f.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	raw, err := os.ReadFile(md5Out)
	require.NoError(t, err)
	line := strings.TrimSpace(string(raw))
	// Must be "<32hexchars>  <path>" with exactly two spaces.
	assert.Regexp(t, `^[0-9a-f]{32}  opt/f\.txt$`, line)
}

func TestNoMd5sumsOutputFlag(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "f.txt", "data")
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "") // no md5sums output
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsFile, Dest: "f.txt", Src: src},
	})

	require.NoError(t, run(cfg))

	// tar should exist; no md5sums file should be created.
	_, err := os.Stat(out)
	assert.NoError(t, err)
}

// indexOf returns the position of name in names, or -1 if absent.
func indexOf(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

// TestAddDir_AutoCreatesParents verifies that a "dir" manifest entry whose
// parent directories do not yet exist in the archive causes those parents to be
// written first when create_parents is set.
func TestAddDir_AutoCreatesParents(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tar")

	cfg := defaultConfig(out, "")
	cfg.CreateParents = true
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsDir, Dest: "a/b/c"},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	names := make([]string, len(hdrs))
	for i, h := range hdrs {
		names[i] = h.Name
	}

	require.Contains(t, names, "a/")
	require.Contains(t, names, "a/b/")
	require.Contains(t, names, "a/b/c/")
	assert.Less(t, indexOf(names, "a/"), indexOf(names, "a/b/"), "a/ must precede a/b/")
	assert.Less(t, indexOf(names, "a/b/"), indexOf(names, "a/b/c/"), "a/b/ must precede a/b/c/")
}

// TestParentOrdering_InstallerLike reproduces the sort-order regression seen
// with the installer package.  The manifest has two dir entries that sort
// before a file entry; the dir entries introduce an implied parent
// (embedded/) that was not otherwise listed.  Without the fix, embedded/
// appears after its children embedded/bin/ and embedded/lib/.
//
// Manifest (as rules_pkg write_manifest would sort it):
//
//	opt/datadog-installer/embedded/bin   ← dir (from pkg_mkdirs)
//	opt/datadog-installer/embedded/lib   ← dir (from pkg_mkdirs)
//	opt/datadog-installer/version-manifest.json  ← file
func TestParentOrdering_InstallerLike(t *testing.T) {
	dir := t.TempDir()
	vmanifest := writeFile(t, dir, "version-manifest.json", `{"version":"7"}`)
	out := filepath.Join(dir, "out.tar")
	md5Out := filepath.Join(dir, "out.md5sums")

	cfg := defaultConfig(out, md5Out)
	cfg.CreateParents = true
	cfg.ManifestPath = filepath.Join(dir, "manifest.json")
	// Entries in the order write_manifest would produce (lexicographic by dest).
	writeManifest(t, cfg.ManifestPath, []ManifestEntry{
		{Type: entryIsDir, Dest: "opt/datadog-installer/embedded/bin"},
		{Type: entryIsDir, Dest: "opt/datadog-installer/embedded/lib"},
		{Type: entryIsFile, Dest: "opt/datadog-installer/version-manifest.json", Src: vmanifest},
	})

	require.NoError(t, run(cfg))

	hdrs := tarEntries(t, out)
	names := make([]string, len(hdrs))
	for i, h := range hdrs {
		names[i] = h.Name
	}

	// All expected entries are present.
	assert.Contains(t, names, "opt/")
	assert.Contains(t, names, "opt/datadog-installer/")
	assert.Contains(t, names, "opt/datadog-installer/embedded/")
	assert.Contains(t, names, "opt/datadog-installer/embedded/bin/")
	assert.Contains(t, names, "opt/datadog-installer/embedded/lib/")
	assert.Contains(t, names, "opt/datadog-installer/version-manifest.json")

	// Parent directories precede their children.
	assert.Less(t, indexOf(names, "opt/"), indexOf(names, "opt/datadog-installer/"),
		"opt/ must precede opt/datadog-installer/")
	assert.Less(t, indexOf(names, "opt/datadog-installer/"), indexOf(names, "opt/datadog-installer/embedded/"),
		"opt/datadog-installer/ must precede opt/datadog-installer/embedded/")
	assert.Less(t, indexOf(names, "opt/datadog-installer/embedded/"), indexOf(names, "opt/datadog-installer/embedded/bin/"),
		"embedded/ must precede embedded/bin/")
	assert.Less(t, indexOf(names, "opt/datadog-installer/embedded/"), indexOf(names, "opt/datadog-installer/embedded/lib/"),
		"embedded/ must precede embedded/lib/")

	// md5sums contains only the regular file, not directories.
	sums := parseMd5sums(t, md5Out)
	assert.Contains(t, sums, "opt/datadog-installer/version-manifest.json")
	assert.Len(t, sums, 1)
}

func TestParseFlags_RequiresOutput(t *testing.T) {
	_, err := parseFlags([]string{"--manifest", "m.json"})
	assert.ErrorContains(t, err, "--output")
}

func TestParseFlags_InvalidMode(t *testing.T) {
	_, err := parseFlags([]string{"--output", "out.tar", "--mode", "9999"})
	assert.Error(t, err)
}

func TestParseFlags_InvalidOwner(t *testing.T) {
	_, err := parseFlags([]string{"--output", "out.tar", "--owner", "notanumber"})
	assert.Error(t, err)
}
