// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package xargs_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// replaceAllocLimit is far above what a rejected -I invocation needs, and far
// below what building the oversized command line would allocate (hundreds of
// MiB in each test below).
const replaceAllocLimit = 64 << 20

// allocDuring returns the bytes allocated process-wide while f runs.
func allocDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// VULN-4556: an -I template that repeats the marker must be rejected by the
// -s check before the substituted command line is built, and before the
// nested command is looked up.
func TestXargsReplaceRepeatedMarkerRejectedBeforeSubstitution(t *testing.T) {
	dir := t.TempDir()
	item := strings.Repeat("a", 128<<10)
	template := strings.Repeat("{}", 2000) // 2000 x 128 KiB = 250 MiB if built
	script := "echo " + item + " | xargs -I{} -s 131072 echo " + template

	var stdout, stderr string
	var code int
	alloc := allocDuring(func() {
		stdout, stderr, code = runScript(t, script, dir,
			interp.AllowedCommands([]string{"rshell:xargs", "rshell:echo"}))
	})
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout, "the nested command must not run")
	assert.Equal(t, "xargs: argument line too long\n", stderr)
	assert.Less(t, alloc, uint64(replaceAllocLimit), "oversized -I command line was materialized")
}

// VULN-1678: the same amplification from a small script that grows the item
// by doubling a variable, with the default -s and a single-byte marker.
func TestXargsReplaceSmallScriptAmplificationRejected(t *testing.T) {
	dir := t.TempDir()
	var script strings.Builder
	script.WriteString("A=AAAAAAAAAAAAAAAA\n")
	for range 15 {
		script.WriteString("A=$A$A\n") // 16 B * 2^15 = 512 KiB
	}
	script.WriteString("echo \"$A\" | xargs -I @ echo " + strings.Repeat("@", 512) + "\n") // 256 MiB if built

	var stdout, stderr string
	var code int
	alloc := allocDuring(func() {
		stdout, stderr, code = runScript(t, script.String(), dir,
			interp.AllowedCommands([]string{"rshell:xargs", "rshell:echo"}))
	})
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "xargs: argument line too long\n", stderr)
	assert.Less(t, alloc, uint64(replaceAllocLimit), "oversized -I command line was materialized")
}

// The pre-substitution size check must count exactly what the substituted
// command line holds: a line of exactly -s characters still runs, one more
// is rejected.
func TestXargsReplaceSizeCheckIsExact(t *testing.T) {
	dir := t.TempDir()
	// "echo" + NUL + "[xyz][xyz]" + NUL = 5 + 11 = 16 bytes.
	stdout, stderr, code := cmdRun(t, "echo xyz | xargs -I{} -s 16 echo [{}][{}]", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "[xyz][xyz]\n", stdout)

	stdout, stderr, code = cmdRun(t, "echo xyz | xargs -I{} -s 15 echo [{}][{}]", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "xargs: argument line too long\n", stderr)
}

// Ordinary -I substitutions, including a marker in the command name, an
// empty item, and a template without the marker, are unaffected.
func TestXargsReplaceNormalSubstitutionUnaffected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "printf 'a\\nbb\\n' | xargs -I{} echo pre-{}-{}-post", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "pre-a-a-post\npre-bb-bb-post\n", stdout)

	stdout, stderr, code = cmdRun(t, "echo echo | xargs -I CMD CMD hello CMD", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "hello echo\n", stdout)

	stdout, stderr, code = cmdRun(t, "echo x | xargs -I{} echo no marker", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "no marker\n", stdout)
}
