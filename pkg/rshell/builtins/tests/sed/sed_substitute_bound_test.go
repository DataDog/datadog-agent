// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// substituteAllocLimit is far above what a rejected substitution needs, and
// far below what building the oversized result would allocate (hundreds of
// MiB in each test below).
const substituteAllocLimit = 64 << 20

// assertBoundedAlloc fails if alloc reaches substituteAllocLimit. The check
// is skipped under the race detector, which randomly drops sync.Pool
// entries so regexp reallocates its pooled matcher state on every match.
func assertBoundedAlloc(t *testing.T, alloc uint64) {
	t.Helper()
	if raceEnabled {
		return
	}
	assert.Less(t, alloc, uint64(substituteAllocLimit), "oversized s/// result was materialized")
}

// allocDuring returns the bytes allocated process-wide while f runs.
func allocDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func runSedOnly(t *testing.T, script string) (string, string, int, uint64) {
	t.Helper()
	dir := t.TempDir()
	var stdout, stderr string
	var code int
	alloc := allocDuring(func() {
		stdout, stderr, code = runScript(t, script, dir,
			interp.AllowedCommands([]string{"rshell:sed"}))
	})
	return stdout, stderr, code, alloc
}

// VULN-4545: a long replacement applied to every byte of a line must be
// rejected before the result is built.
func TestSedSubstituteAmplifiedReplacementRejectedBeforeBuild(t *testing.T) {
	line := strings.Repeat("x", 32<<10)
	repl := strings.Repeat("Y", 4096) // 32 Ki matches x 4 KiB = 128 MiB if built
	script := "sed 's/./" + repl + "/g' <<'EOF'\n" + line + "\nEOF\n"

	stdout, stderr, code, alloc := runSedOnly(t, script)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "sed: standard input: pattern space exceeded size limit\n", stderr)
	assertBoundedAlloc(t, alloc)
}

// VULN-1677: chained substitutions that grow the pattern space up to the
// limit must not let the next step build a result far beyond it.
func TestSedSubstituteChainedGrowthRejectedBeforeBuild(t *testing.T) {
	x1024 := strings.Repeat("x", 1024)
	x200 := strings.Repeat("x", 200) // 1 MiB x 200 = 200 MiB if built
	script := "sed 's/x/" + x1024 + "/; s/x/" + x1024 + "/g; s/x/" + x200 + "/g' <<'EOF'\nx\nEOF\n"

	stdout, stderr, code, alloc := runSedOnly(t, script)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "sed: standard input: pattern space exceeded size limit\n", stderr)
	assertBoundedAlloc(t, alloc)
}

// A substitution whose result is exactly the 1 MiB limit still succeeds,
// and one byte more is rejected. Both lines are long enough to need the
// exact sizing walk rather than the static bound.
func TestSedSubstituteExactLimitBoundary(t *testing.T) {
	half := strings.Repeat("a", 512<<10)

	stdout, stderr, code, _ := runSedOnly(t, "sed 's/a/bb/g' <<'EOF'\n"+half+"\nEOF\n")
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, strings.Repeat("b", 1<<20)+"\n", stdout)

	_, stderr, code, _ = runSedOnly(t, "sed 's/a/bb/g' <<'EOF'\n"+half+"a\nEOF\n")
	assert.Equal(t, 1, code)
	assert.Equal(t, "sed: standard input: pattern space exceeded size limit\n", stderr)
}

// Substitutions on long lines whose results stay small are unaffected,
// including group references and anchors that the sizing handles with an
// upper bound or a re-match.
func TestSedSubstituteLongLineSmallResultUnaffected(t *testing.T) {
	line := "a" + strings.Repeat("b", 900<<10)
	for _, tc := range []struct {
		script, want string
	}{
		{`s/\(a\)b*/\1\1\1/g`, "aaa\n"},
		{`s/^a.*$/short/`, "short\n"},
		{`s/^.*/[&]/2`, line + "\n"},
		{`s/b*$/END/g`, "aEND\n"},
		{`s/^\(a\)b*/\1\1\1/g`, "aaa\n"},
		{`s/^\(a\)b*/\1\1\1/`, "aaa\n"},
	} {
		stdout, stderr, code, _ := runSedOnly(t, "sed '"+tc.script+"' <<'EOF'\n"+line+"\nEOF\n")
		assert.Equal(t, 0, code, "%s: stderr=%q", tc.script, stderr)
		assert.Equal(t, tc.want, stdout, tc.script)
	}
}

// A match whose assertion only holds in context is re-matched in isolation
// by the non-global paths, which can then replace every remaining byte of
// it separately. The sizing must charge those replacements, not one.
func TestSedSubstituteIsolatedRematchRejectedBeforeBuild(t *testing.T) {
	line := "xa" + strings.Repeat("b", 32<<10)
	repl := strings.Repeat("Y", 4096) // 32 Ki pieces x 4 KiB = 128 MiB if built
	for _, flags := range []string{"", "1", "1g"} {
		script := "sed -E 's/\\Ba.*|b/" + repl + "/" + flags + "' <<'EOF'\n" + line + "\nEOF\n"

		stdout, stderr, code, alloc := runSedOnly(t, script)
		assert.Equal(t, 1, code, "flags %q", flags)
		assert.Empty(t, stdout, "flags %q", flags)
		assert.Equal(t, "sed: standard input: pattern space exceeded size limit\n", stderr, "flags %q", flags)
		assertBoundedAlloc(t, alloc)
	}
}

// An escaped dollar is a literal byte pair, so repeating \$1 must neither
// copy the input nor be sized as if it did.
func TestSedSubstituteEscapedDollarStaysLiteral(t *testing.T) {
	line := strings.Repeat("a", 32<<10)
	repl := strings.Repeat(`\$1`, 512) // 16 MiB if \$1 copied the line
	script := "sed -E 's/(.*)/" + repl + "/g' <<'EOF'\n" + line + "\nEOF\n"

	stdout, stderr, code, alloc := runSedOnly(t, script)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, strings.Repeat("$1", 512)+"\n", stdout)
	assertBoundedAlloc(t, alloc)
}
