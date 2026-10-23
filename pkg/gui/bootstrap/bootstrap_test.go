// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bootstrap

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useTempDir points os.TempDir() at a directory owned by the test.
func useTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	// TMPDIR is what os.TempDir reads on Unix, TMP on Windows.
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	require.Equal(t, dir, os.TempDir())

	return dir
}

// pathOf turns the file:// URL returned by Write back into a local path.
func pathOf(t *testing.T, pageURL string) string {
	t.Helper()

	u, err := url.Parse(pageURL)
	require.NoError(t, err)
	require.Equal(t, "file", u.Scheme)
	require.Empty(t, u.Host, "a file URL must have an empty authority")

	path := u.Path
	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
	}

	return filepath.FromSlash(path)
}

func TestWrite(t *testing.T) {
	useTempDir(t)

	tok := Token{ID: "an-id", Secret: "a-secret"}
	pageURL, err := Write("localhost:5002", tok)
	require.NoError(t, err)

	pagePath := pathOf(t, pageURL)
	assert.Equal(t, pageName, filepath.Base(pagePath))
	assert.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(pagePath)), dirPrefix))

	page, err := os.ReadFile(pagePath)
	require.NoError(t, err)

	content := string(page)
	assert.Contains(t, content, `action="http://localhost:5002/auth"`)
	assert.Contains(t, content, `method="POST"`)
	assert.Contains(t, content, `name="id" value="an-id"`)
	assert.Contains(t, content, `name="secret" value="a-secret"`)
	assert.Contains(t, content, `content="no-referrer"`)
}

func TestWriteRestrictsAccessToCurrentUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not an access control mechanism on Windows; the per-user temporary directory ACL covers the file there")
	}

	useTempDir(t)

	pageURL, err := Write("localhost:5002", Token{ID: "an-id", Secret: "a-secret"})
	require.NoError(t, err)

	pagePath := pathOf(t, pageURL)

	pageInfo, err := os.Stat(pagePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), pageInfo.Mode().Perm(), "the browser must be the only reader of the intent token")

	dirInfo, err := os.Stat(filepath.Dir(pagePath))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), dirInfo.Mode().Perm())
}

func TestWriteEscapesTheToken(t *testing.T) {
	useTempDir(t)

	pageURL, err := Write("localhost:5002", Token{ID: `"><script>alert(1)</script>`, Secret: "a-secret"})
	require.NoError(t, err)

	page, err := os.ReadFile(pathOf(t, pageURL))
	require.NoError(t, err)

	assert.NotContains(t, string(page), "<script>alert(1)</script>")
}

func TestWriteIsUnpredictable(t *testing.T) {
	useTempDir(t)

	first, err := Write("localhost:5002", Token{ID: "an-id", Secret: "a-secret"})
	require.NoError(t, err)

	second, err := Write("localhost:5002", Token{ID: "an-id", Secret: "a-secret"})
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestSweep(t *testing.T) {
	tempDir := useTempDir(t)

	aged := filepath.Join(tempDir, dirPrefix+"aged")
	require.NoError(t, os.Mkdir(aged, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(aged, pageName), []byte("stale"), 0600))
	require.NoError(t, os.Chtimes(aged, time.Now(), time.Now().Add(-2*sweepAge)))

	unrelated := filepath.Join(tempDir, "something-else")
	require.NoError(t, os.Mkdir(unrelated, 0700))
	require.NoError(t, os.Chtimes(unrelated, time.Now(), time.Now().Add(-2*sweepAge)))

	fresh, err := Write("localhost:5002", Token{ID: "an-id", Secret: "a-secret"})
	require.NoError(t, err)

	Sweep()

	assert.NoDirExists(t, aged, "an aged bootstrap directory should be swept")
	assert.DirExists(t, unrelated, "Sweep must only remove its own directories")
	assert.FileExists(t, pathOf(t, fresh), "the launch in progress must not be swept")
}

func TestSweepToleratesAMissingTempDir(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("TMP", filepath.Join(t.TempDir(), "does-not-exist"))

	Sweep()
}
