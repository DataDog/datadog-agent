// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// execAllocLimit is far above what a rejected -exec needs, and far below
// what building the oversized command line would allocate (about 300 MiB in
// each test below).
const execAllocLimit = 64 << 20

// allocDuring returns the bytes allocated process-wide while f runs.
func allocDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// longStartPath returns a starting point of about 1 KiB that names target.
// find substitutes the starting point verbatim for {}.
func longStartPath() string {
	return strings.Repeat("./", 500) + "target"
}

// VULN-4541: a command template that repeats {} must be rejected before the
// generated command is built, whether {} sits in the command name (which
// was expanded before the allowlist check) or in the arguments (which had
// no size check at all).
func TestFindExecRepeatedPlaceholderRejectedBeforeSubstitution(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), nil, 0o644))
	// Quoted so the interpreter passes the template to find as one literal
	// word rather than brace-expanding it.
	markers := "'" + strings.Repeat("{}", 300_000) + "'" // 300k x ~1 KiB = ~300 MiB if built

	for _, tc := range []struct {
		name    string
		script  string
		allowed []string
	}{
		{
			name:    "command name, nested command not allowed",
			script:  "find " + longStartPath() + " -exec " + markers + " \\;",
			allowed: []string{"rshell:find"},
		},
		{
			name:    "arguments, nested command allowed",
			script:  "find " + longStartPath() + " -exec echo " + markers + " \\;",
			allowed: []string{"rshell:find", "rshell:echo"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr string
			var code int
			alloc := allocDuring(func() {
				stdout, stderr, code = testutil.RunScript(t, tc.script, dir,
					interp.AllowedPaths([]string{dir}),
					interp.AllowedCommands(tc.allowed))
			})
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout, "the nested command must not run")
			assert.Equal(t, "find: -exec: argument list too long\n", stderr)
			assert.Less(t, alloc, uint64(execAllocLimit), "oversized -exec command line was materialized")
		})
	}
}

// Ordinary -exec and -execdir substitutions are unaffected.
func TestFindExecNormalSubstitutionUnaffected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "f.txt"), nil, 0o644))
	opts := []interp.RunnerOption{
		interp.AllowedPaths([]string{dir}),
		interp.AllowedCommands([]string{"rshell:find", "rshell:echo"}),
	}

	stdout, stderr, code := testutil.RunScript(t, "find sub -name f.txt -exec echo [{}] {}{} \\;", dir, opts...)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "[sub/f.txt] sub/f.txtsub/f.txt\n", stdout)

	stdout, stderr, code = testutil.RunScript(t, "find sub -name f.txt -execdir echo {} \\;", dir, opts...)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "./f.txt\n", stdout)
}
