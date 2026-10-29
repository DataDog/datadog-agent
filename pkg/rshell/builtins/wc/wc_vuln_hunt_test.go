// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package wc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// Campaign: vuln-hunt/2026-05-19-codex. These are public-safe blocked-attack
// regressions for wc.

func TestVulnHuntBuiltinFileAccessBypass_SymlinkOperandOutsideAllowedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires platform-specific privileges on Windows")
	}

	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret words\n"), 0644))
	require.NoError(t, os.Symlink(filepath.Join(secret, "secret.txt"), filepath.Join(allowed, "escape_link")))

	stdout, stderr, code := runScript(t, "wc escape_link", allowed, interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "wc:")
	assert.NotContains(t, stdout+stderr, "secret words")
}

func TestVulnHuntBuiltinResourceExhaustion_ManyFileOperandsCloseHandles(t *testing.T) {
	dir := t.TempDir()
	args := make([]string, 0, 200)
	for i := range 200 {
		name := fmt.Sprintf("f%03d.txt", i)
		writeFile(t, dir, name, "x\n")
		args = append(args, name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stdout, stderr, code := runScriptCtx(ctx, t, "wc "+strings.Join(args, " "), dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "total")
	assert.Contains(t, stdout, "200")
}

func TestVulnHuntBuiltinIntegerOverflow_LargeBoundedCountsAndWidths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "large.txt", strings.Repeat("word\n", 100_000))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stdout, stderr, code := runScriptCtx(ctx, t, "wc -lwmcL large.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "100000")
	assert.Contains(t, stdout, "500000")
	assert.Contains(t, stdout, "large.txt")
}

func TestVulnHuntBuiltinSpecialFiles_RepeatedDashDoesNotCorruptState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "alpha\nbeta\n")

	stdout, stderr, code := cmdRun(t, "cat input.txt | wc - -", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "-")
	assert.Contains(t, stdout, "total")
}
