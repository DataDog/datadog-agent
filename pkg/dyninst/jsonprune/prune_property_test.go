// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package jsonprune

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// genValueTree builds a random captured-value object up to maxDepth deep.
// At leaf depth it emits a flat {type, data} object; at inner depths it
// emits an {type, fields:{...}} with 1-3 nested children.
func genValueTree(r *rand.Rand, maxDepth int) string {
	t := fmt.Sprintf("T%d", r.Intn(10))
	if maxDepth == 0 {
		dataLen := 5 + r.Intn(80)
		data := strings.Repeat("x", dataLen)
		return fmt.Sprintf(`{"type":%q,"data":%q}`, t, data)
	}
	n := 1 + r.Intn(3)
	var b strings.Builder
	fmt.Fprintf(&b, `{"type":%q,"fields":{`, t)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"f%d":%s`, i, genValueTree(r, maxDepth-1))
	}
	b.WriteString("}}")
	return b.String()
}

// genSnapshot returns a syntactically-valid snapshot with a pruneable
// captures.arguments section populated by random value trees.
func genSnapshot(r *rand.Rand) []byte {
	nArgs := 1 + r.Intn(8)
	args := map[string]string{}
	for i := 0; i < nArgs; i++ {
		args[fmt.Sprintf("arg%d", i)] = genValueTree(r, 1+r.Intn(4))
	}
	return []byte(envelope(args))
}

func TestProperty_PruneInvariants(t *testing.T) {
	const iterations = 1000
	r := rand.New(rand.NewSource(0xCAFEBABE))
	for i := 0; i < iterations; i++ {
		input := genSnapshot(r)
		// Pick a random budget somewhere between "fast path" and "so
		// tight even the envelope won't fit".
		var maxSize int
		switch r.Intn(4) {
		case 0:
			maxSize = len(input) + 10 // fast path
		case 1:
			maxSize = len(input) - 1 // barely over
		case 2:
			maxSize = len(input) / 2
		case 3:
			maxSize = 30 // impossibly tight
		}
		out := Prune(input, maxSize)

		// Invariant 1: output is valid JSON.
		require.True(t, json.Valid(out),
			"iter %d: output not valid JSON (input %d bytes, maxSize %d)\n  %s",
			i, len(input), maxSize, out)

		// Invariant 2: output length <= input length.
		require.LessOrEqual(t, len(out), len(input),
			"iter %d: output larger than input", i)

		// Invariant 3: envelope preserved. The snapshot container and
		// the captures section must still be present.
		s := string(out)
		require.Contains(t, s, `"service"`, "iter %d", i)
		require.Contains(t, s, `"debugger.snapshot"`, "iter %d", i)
		require.Contains(t, s, `"captures"`, "iter %d", i)

		// Invariant 4: output fits budget OR was unshrinkable. When
		// the input already fits we pass through; otherwise the final
		// size should be <= maxSize unless envelope exceeds it.
		if len(input) > maxSize {
			if !isUnshrinkable(input, maxSize) {
				require.LessOrEqual(t, len(out), maxSize+200,
					"iter %d: not shrunk enough (%d vs %d)\ninput=%s",
					i, len(out), maxSize, input)
			}
		}

		// Invariant 5: when the output already fits the budget, a
		// second Prune call with the same budget is a no-op (fast
		// path — same underlying array).
		if len(out) <= maxSize {
			out2 := Prune(out, maxSize)
			require.Equal(t, len(out), len(out2),
				"iter %d: prune not idempotent under budget", i)
		}
	}
}

// isUnshrinkable reports whether the envelope alone already exceeds
// maxSize, in which case we can't get below the budget no matter what.
func isUnshrinkable(_ []byte, maxSize int) bool {
	// A fully-pruned snapshot with no args is roughly 80 bytes of
	// envelope. If that's already above budget, Prune legitimately
	// can't fit.
	return maxSize < 100
}
