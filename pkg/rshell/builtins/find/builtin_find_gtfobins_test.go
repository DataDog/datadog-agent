// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func findGTFORun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// --- GTFOBins validation ---

// TestFindGTFOBinsExecShellBlocked verifies that the GTFOBins shell-escape
// technique for find (-exec /bin/sh) is blocked because /bin/sh is not an
// allowed command in the restricted shell.
//
// GTFOBins: https://gtfobins.org/gtfobins/find/
// Technique: find . -exec /bin/sh \; -quit
func TestFindGTFOBinsExecShellBlocked(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644))
	_, stderr, code := findGTFORun(t, `find . -exec /bin/sh \;`, dir)
	assert.NotEqual(t, 0, code)
	assert.NotEmpty(t, stderr)
}

// TestFindGTFOBinsFprintfBlocked verifies that the GTFOBins file-write
// technique for find (-fprintf) is blocked during expression parsing.
//
// GTFOBins: https://gtfobins.org/gtfobins/find/
// Technique: find . -fprintf /path/to/output %p
func TestFindGTFOBinsFprintfBlocked(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := findGTFORun(t, `find . -fprintf /tmp/evil %p`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

// TestFindGTFOBinsDeleteBlocked verifies that -delete is blocked.
//
// GTFOBins: https://gtfobins.org/gtfobins/find/
// Technique: find . -delete
func TestFindGTFOBinsDeleteBlocked(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := findGTFORun(t, `find . -delete`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

// TestFindGTFOBinsSandboxEscape verifies that find cannot traverse outside
// the AllowedPaths sandbox.
//
// GTFOBins: https://gtfobins.org/gtfobins/find/
// Technique: find /path/to/secret -name '*'
func TestFindGTFOBinsSandboxEscape(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret\n"), 0644))
	secretPath := strings.ReplaceAll(secret, `\`, `/`)
	_, stderr, code := findGTFORun(t, "find "+secretPath+" -name '*'", allowed)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "find:")
}
