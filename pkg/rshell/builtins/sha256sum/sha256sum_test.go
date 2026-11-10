// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sha256sum_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sha256builtin "github.com/DataDog/datadog-agent/pkg/rshell/builtins/sha256sum"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

const (
	emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	abcDigest   = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
)

func runSHA256Sum(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

func runAllowed(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runSHA256Sum(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0644))
}

func TestKnownVectorsAndRawBytes(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		digest  string
	}{
		{name: "empty", digest: emptyDigest},
		{name: "abc", content: []byte("abc"), digest: abcDigest},
		{name: "crlf", content: []byte("abc\r\n"), digest: "552bab6864c7a7b69a502ed1854b9245c0e1a30f008aaa0b281da62585fdb025"},
		{name: "binary", content: []byte{0, 1, 0xff, '\r', '\n'}, digest: "5b67fc778b067e90ba58cc6b058f2295d0e48f1e8a23651a78b73ec256d8eb49"},
		{name: "million-a", content: []byte(strings.Repeat("a", 1_000_000)), digest: "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tt.name, tt.content)
			stdout, stderr, code := runAllowed(t, "sha256sum "+tt.name, dir)
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.Equal(t, tt.digest+"  "+tt.name+"\n", stdout)
		})
	}
}

func TestStdinAndMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty", nil)
	writeFile(t, dir, "abc", []byte("abc"))

	stdout, stderr, code := runAllowed(t, "printf abc | sha256sum", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, abcDigest+"  -\n", stdout)

	stdout, stderr, code = runAllowed(t, "sha256sum empty abc", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, emptyDigest+"  empty\n"+abcDigest+"  abc\n", stdout)
}

func TestGenerationContinuesAfterMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))

	stdout, stderr, code := runAllowed(t, "sha256sum missing abc", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, abcDigest+"  abc\n", stdout)
	assert.Contains(t, stderr, "sha256sum: missing:")
}

func TestCheckManifestFormats(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))
	manifest := strings.Join([]string{
		abcDigest + "  abc",
		strings.ToUpper(abcDigest) + " *abc",
		"SHA256 (abc) = " + abcDigest,
	}, "\n") + "\n"
	writeFile(t, dir, "checksums", []byte(manifest))

	stdout, stderr, code := runAllowed(t, "sha256sum -c checksums", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "abc: OK\nabc: OK\nabc: OK\n", stdout)
}

func TestCheckManifestLineEndingsAndWhitespace(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(ending, "\r", "CR"), func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "abc", []byte("abc"))
			writeFile(t, dir, "checksums", []byte(" \t"+abcDigest+"\tabc"+ending))

			stdout, stderr, code := runAllowed(t, "sha256sum -c checksums", dir)
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.Equal(t, "abc: OK\n", stdout)
		})
	}

	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))
	writeFile(t, dir, "checksums", []byte(abcDigest+"  abc\n"+abcDigest+"  abc\r\n"+abcDigest+"  abc\r"))
	stdout, stderr, code := runAllowed(t, "sha256sum -c checksums", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "abc: OK\nabc: OK\nabc: OK\n", stdout)
}

func TestCheckFromStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))
	script := "printf '%s\\n' '" + abcDigest + "  abc' | sha256sum --check"

	stdout, stderr, code := runAllowed(t, script, dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "abc: OK\n", stdout)
}

func TestDashTargetDependsOnManifestSource(t *testing.T) {
	dir := t.TempDir()
	script := "printf '%s\\n' '" + emptyDigest + "  -' | sha256sum -c -"
	stdout, stderr, code := runAllowed(t, script, dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "no properly formatted checksum lines found")

	writeFile(t, dir, "checksums", []byte(emptyDigest+"  -\n"))
	stdout, stderr, code = runAllowed(t, "sha256sum -c checksums", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "-: OK\n", stdout)
}

func TestEscapedFilenameRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path separators cannot be embedded in a filename")
	}
	dir := t.TempDir()
	const name = "a\\b"
	writeFile(t, dir, name, []byte("abc"))

	manifest, stderr, code := runAllowed(t, "sha256sum 'a\\b'", dir)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Equal(t, "\\"+abcDigest+"  a\\\\b\n", manifest)
	writeFile(t, dir, "checksums", []byte(manifest))

	stdout, stderr, code := runAllowed(t, "sha256sum -c checksums", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "\\a\\\\b: OK\n", stdout)
}

func TestCheckMismatchAndUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("changed"))
	writeFile(t, dir, "checksums", []byte(abcDigest+"  abc\n"+emptyDigest+"  missing\n"))

	stdout, stderr, code := runAllowed(t, "sha256sum -c checksums", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "abc: FAILED\nmissing: FAILED open or read\n", stdout)
	assert.Contains(t, stderr, "sha256sum: missing:")
	assert.Contains(t, stderr, "WARNING: 1 listed file could not be read")
	assert.Contains(t, stderr, "WARNING: 1 computed checksum did NOT match")
}

func TestQuietAndStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))
	writeFile(t, dir, "good", []byte(abcDigest+"  abc\n"))
	writeFile(t, dir, "bad", []byte(emptyDigest+"  abc\n"))

	stdout, stderr, code := runAllowed(t, "sha256sum -c --quiet good", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)

	stdout, stderr, code = runAllowed(t, "sha256sum -c --status bad", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestMalformedManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc", []byte("abc"))
	writeFile(t, dir, "mixed", []byte("not a checksum\n"+abcDigest+"  abc\n"))
	writeFile(t, dir, "invalid", []byte("not a checksum\n"))

	stdout, stderr, code := runAllowed(t, "sha256sum -c mixed", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc: OK\n", stdout)
	assert.Contains(t, stderr, "WARNING: 1 line is improperly formatted")

	stdout, stderr, code = runAllowed(t, "sha256sum -c invalid", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "no properly formatted checksum lines found")
}

func TestCheckOptionsRequireCheckMode(t *testing.T) {
	for _, flag := range []string{"--quiet", "--status"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, code := runAllowed(t, "sha256sum "+flag, t.TempDir())
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "meaningful only when verifying checksums")
		})
	}
}

func TestUnsupportedGNUFlagsAreRejected(t *testing.T) {
	for _, flag := range []string{"-b", "-t", "-w", "-z", "--binary", "--text", "--tag", "--zero", "--ignore-missing", "--strict", "--warn", "--version"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, code := runAllowed(t, "sha256sum "+flag, t.TempDir())
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.NotEmpty(t, stderr)
		})
	}
}

func TestNoArgumentFlagsRejectExplicitValues(t *testing.T) {
	for _, flag := range []string{"--check=false", "--quiet=false", "--status=false", "--help=false"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, code := runAllowed(t, "sha256sum "+flag, t.TempDir())
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "doesn't allow an argument")
		})
	}
}

func TestOperandAndManifestEntryLimits(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runAllowed(t, "sha256sum"+strings.Repeat(" -", sha256builtin.MaxOperands+1), dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "too many operands")

	writeFile(t, dir, "abc", []byte("abc"))
	line := abcDigest + "  abc\n"
	writeFile(t, dir, "checksums", []byte(strings.Repeat(line, sha256builtin.MaxManifestEntries)))
	stdout, stderr, code = runAllowed(t, "sha256sum -c --status checksums", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)

	writeFile(t, dir, "checksums", []byte(strings.Repeat(line, sha256builtin.MaxManifestEntries+1)))
	stdout, stderr, code = runAllowed(t, "sha256sum -c --status checksums", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "entry limit")
}

func TestBlockedStdinHonorsCancellation(t *testing.T) {
	for _, script := range []string{"sha256sum", "sha256sum -c"} {
		t.Run(script, func(t *testing.T) {
			dir := t.TempDir()
			reader, writer, err := os.Pipe()
			require.NoError(t, err)
			defer reader.Close()
			defer writer.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			testutil.RunScriptCtx(ctx, t, script, dir,
				interp.StdIO(reader, io.Discard, io.Discard),
				interp.AllowedPaths([]string{dir}),
			)
			assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			assert.Less(t, time.Since(start), 2*time.Second)
		})
	}
}

func TestRejectsNonRegularAndDisallowedFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory"), 0755))
	writeFile(t, dir, "secret", []byte("secret"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "allowed"), 0755))

	stdout, stderr, code := runAllowed(t, "sha256sum directory", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "sha256sum: directory:")

	stdout, stderr, code = runSHA256Sum(t, "sha256sum secret", dir, interp.AllowedPaths([]string{filepath.Join(dir, "allowed")}))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "permission denied")
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		stdout, stderr, code := runAllowed(t, "sha256sum "+flag, t.TempDir())
		assert.Equal(t, 0, code)
		assert.Empty(t, stderr)
		assert.Contains(t, stdout, "Usage: sha256sum")
		assert.Contains(t, stdout, "--check")
		assert.NotContains(t, stdout, "\x00")
		assert.NotContains(t, stdout, "[=")
	}
}
