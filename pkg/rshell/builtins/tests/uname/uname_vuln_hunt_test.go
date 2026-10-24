// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uname_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntUnameRejectsExpandedOperandAsProcName(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())

	stdout, stderr, code := cmdRun(t, `target=hostname; uname "$target"`, dir)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "uname: extra operand 'hostname'")
	assert.NotContains(t, stdout, "testhost")
}

func TestVulnHuntUnameOutputRedirectValidationRunsBeforeProcRead(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, map[string]string{"ostype": "SHOULD_NOT_PRINT"})

	stdout, stderr, code := cmdRun(t, "uname > created.txt", dir)

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "> file redirection is not supported")
	assert.NoFileExists(t, filepath.Join(dir, "created.txt"))
}

func TestVulnHuntUnameAllOutputStaysBoundedByProcReader(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	kernelDir := filepath.Join(dir, "sys", "kernel")
	require.NoError(t, os.MkdirAll(kernelDir, 0o755))
	huge := strings.Repeat("A", 8192)
	for _, name := range []string{"ostype", "hostname", "osrelease", "version", "arch"} {
		require.NoError(t, os.WriteFile(filepath.Join(kernelDir, name), []byte(huge), 0o644))
	}

	stdout, stderr, code := cmdRun(t, "uname -a", dir)

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.LessOrEqual(t, len(stdout), 5*4096+5)
	assert.Contains(t, stdout, strings.Repeat("A", 4096))
}
