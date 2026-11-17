// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSubstituteFitsMatchesRealSubstitution checks substituteFits against the
// result execSubstitute actually builds, at every limit around that result's
// size. It must never accept a result larger than the limit, and it must
// accept every result that fits.
func TestSubstituteFitsMatchesRealSubstitution(t *testing.T) {
	inputs := []string{
		"",
		"x",
		"aaaa",
		"abcabcabc",
		"foo bar  baz foo",
		"ab ab xab abx",
		"xxx yxx xyx",
		"xabbbb",
		"abbbbbbb",
	}
	scripts := []string{
		"s/a/XYZ/", "s/a/XYZ/g", "s/a/XYZ/2", "s/a/XYZ/2g",
		"s/b*/<&>/g", "s/x*/-/g", "s/x*/-/2", "s/x*/-/2g",
		"s/\\(a\\)\\(b\\)/\\2\\1\\2\\1/g", "s/\\(a\\)b*/\\1\\1\\1/g",
		"s/\\(a\\)\\|\\(x\\)/[\\1\\2]/g", "s/\\(a\\)\\|\\(x\\)/[\\1\\2]/2",
		"s/^a/QQ/", "s/^/>>/g", "s/$/<</g", "s/\\bab/[&]/g", "s/ab\\b/[&]/2g",
		"s/\\Bx*/\\0\\0/g", "s/\\Bx*/\\0\\0/2", "s/\\(\\Bx\\)*/<\\1>/g",
		"s/\\(\\Bx\\)*/<\\1\\1>/", "s/\\(^x*\\)\\|x\\(x*\\)/\\2\\2\\2/g",
		"s/[a-z]*/&&/g", "s/./\\\\&\\$/g", "s/o/\\n/g",
		// An assertion that only holds in context: the isolated re-match
		// of the selected match replaces each remaining byte separately.
		"s/\\Ba.*\\|b/XX/", "s/\\Ba.*\\|b/XX/2", "s/\\Ba.*\\|b/XX/2g",
		// An escaped dollar is literal, never a capture reference.
		"s/\\(.*\\)/\\$1\\$1/g", "s/\\(b\\)/\\$1\\${1}/2g",
		// Anchored group references sized from their real spans.
		"s/^\\(a\\)b*/\\1\\1\\1/g", "s/^\\(a\\)b*/\\1\\1\\1/",
	}
	for _, script := range scripts {
		cmds, err := parseScript(script, false)
		require.NoError(t, err, script)
		require.Len(t, cmds, 1, script)
		cmd := cmds[0]

		for _, in := range inputs {
			eng := &engine{patternSpace: in}
			require.NoError(t, eng.execSubstitute(cmd), "%s on %q", script, in)
			want := len(eng.patternSpace)

			for limit := max(len(in), want-3); limit <= want+3; limit++ {
				got := substituteFits(cmd.subRe, in, cmd, limit)
				name := fmt.Sprintf("%s on %q (result %d bytes) limit %d", script, in, want, limit)
				if want > limit {
					require.False(t, got, "accepted an oversized result: %s", name)
				} else {
					require.True(t, got, "rejected a result that fits: %s", name)
				}
			}
		}
	}
}

// TestSubstituteFitsBoundsSizingAllocation checks that sizing an oversized
// substitution allocates on the order of its input, not its result.
func TestSubstituteFitsBoundsSizingAllocation(t *testing.T) {
	if raceEnabled {
		// The race detector randomly drops sync.Pool entries, so regexp
		// reallocates its pooled matcher state on every match.
		t.Skip("allocation counts are not meaningful under the race detector")
	}
	cmds, err := parseScript("s/./"+strings.Repeat("Y", 4096)+"/g", false)
	require.NoError(t, err)
	ps := strings.Repeat("x", 64<<10) // 256 MiB result if built

	allocs := testing.AllocsPerRun(1, func() {
		require.False(t, substituteFits(cmds[0].subRe, ps, cmds[0], MaxSpaceBytes))
	})
	// The walk's callback returns "" for each match, so its one output
	// buffer stays empty; allocations are the regexp machinery's own.
	require.Less(t, allocs, float64(1000))
}
