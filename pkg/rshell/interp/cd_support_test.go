// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestCdReportsVarStorageCapFailure verifies that when cd's PWD/OLDPWD
// updates exhaust MaxTotalVarsBytes, changeDir surfaces the error
// instead of silently committing the dir change with stale env vars.
//
// Before the fix, changeDir called setVarString (which stashes errors
// on r.exit.code) and returned nil; the cd builtin's Result{} then
// clobbered r.exit.code, so a script could land in a directory where
// $PWD did not match the actual cwd.
func TestCdReportsVarStorageCapFailure(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	require.NoError(t, os.Mkdir(child, 0o755))

	// Pre-fill the env up to the MaxTotalVarsBytes cap. Each of the
	// 1024 variables holds 1024 bytes, totalling 1 MiB exactly — the
	// cap. Any subsequent positive-delta assignment (cd's OLDPWD/PWD)
	// must be rejected by writeEnv.Set, which we want changeDir to
	// surface as a non-zero exit.
	value := strings.Repeat("x", 1024)
	var sb strings.Builder
	for i := range 1024 {
		fmt.Fprintf(&sb, "VAR_%d=%s\n", i, value)
	}
	// At the cap; cd's PWD/OLDPWD assignment must fail. Capture cd's
	// own exit code via $? so we can assert it independent of the
	// final-command exit code that the script reports overall.
	// `pwd` afterwards verifies the failed cd did not mutate the
	// runner's working directory (matches bash: pwd is untouched on a
	// failed cd). Forward slashes keep the script body portable on
	// Windows.
	fmt.Fprintf(&sb, "cd %s\necho CD_EXIT=$?\necho POST_PWD=$(pwd)\n",
		filepath.ToSlash(child))

	stdout, stderr, _ := runScript(t, sb.String(), dir,
		interp.AllowedPaths([]string{dir}))

	assert.Contains(t, stderr, "variable storage limit exceeded",
		"stderr should mention the storage-cap failure")
	assert.Contains(t, stdout, "CD_EXIT=1",
		"cd must surface a non-zero exit when its env update is rejected")
	assert.Contains(t, stdout, "POST_PWD="+dir,
		"a failed cd must not mutate the runner's working directory")
}
