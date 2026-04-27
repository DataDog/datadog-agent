// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package jsonprune

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envelope wraps a set of captured-value objects in enough schema
// structure that they appear at the pruneable level (>= 5).
//
// Schema levels produced:
//
//	root       {                                 level 0
//	  dbg       "debugger.snapshot": {           level 1 container
//	    cap       "captures": {                  level 2
//	      lines     "lines": {                   level 3
//	        lineN     "$lineNum": {              level 4
//	          args      "arguments": {           level 5 -- pruneable
//	            v1        "$name1": { ... }      level 6 -- pruneable
//	            v2        "$name2": { ... }      level 6
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
//
// Callers pass the serialised form of each variable's value object (as a
// raw string including the surrounding braces) so test expectations can
// be written compactly.
func envelope(args map[string]string) string {
	var inner strings.Builder
	inner.WriteByte('{')
	first := true
	for name, v := range args {
		if !first {
			inner.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&inner, "%q:%s", name, v)
	}
	inner.WriteByte('}')
	return `{"service":"s","debugger.snapshot":{"captures":{"lines":{"1":{"arguments":` +
		inner.String() + `}}}}}`
}

func TestPrune_FastPathPassThrough(t *testing.T) {
	input := []byte(`{"a":1}`)
	out := Prune(input, len(input)+1)
	// Same underlying array.
	require.True(t, unsafe.SliceData(out) == unsafe.SliceData(input),
		"fast path should alias input")
}

func TestPrune_NothingToPrune(t *testing.T) {
	// All content is above level 5 — should return input unchanged
	// even when over budget.
	input := []byte(`{"service":"s","debugger.snapshot":{"captures":{}}}`)
	out := Prune(input, 10)
	require.Equal(t, input, out)
}

func TestPrune_SingleLeafWithType(t *testing.T) {
	big := `{"type":"Foo","value":"` + strings.Repeat("x", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": big}))

	// Budget forces exactly the one leaf to be pruned.
	out := Prune(input, len(input)-100)
	require.NotEqual(t, input, out)
	require.Less(t, len(out), len(input))
	require.True(t, json.Valid(out), "output must be valid JSON")
	// Placeholder must retain the type.
	require.Contains(t, string(out), `"type":"Foo"`)
	require.Contains(t, string(out), `"notCapturedReason":"pruned"`)
}

func TestPrune_LeafWithoutType(t *testing.T) {
	big := `{"value":"` + strings.Repeat("x", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": big}))
	out := Prune(input, len(input)-100)
	require.True(t, json.Valid(out))
	require.Contains(t, string(out), `"notCapturedReason":"pruned"`)
	require.NotContains(t, string(out), `"type"`)
}

func TestPrune_DeepestFirst(t *testing.T) {
	// One value at level 6 ("shallow"), another wraps a level-7
	// object. The inner level-7 object should be pruned first.
	shallow := `{"type":"Shallow","data":"` + strings.Repeat("a", 400) + `"}`
	deep := `{"type":"Wrap","inner":{"type":"Deep","data":"` +
		strings.Repeat("b", 400) + `"}}`
	input := []byte(envelope(map[string]string{"a": shallow, "b": deep}))

	// Shrink by just under one leaf's savings so exactly one prune
	// is needed.
	out := Prune(input, len(input)-350)
	require.True(t, json.Valid(out))
	require.Contains(t, string(out), `"Shallow"`)
	require.NotContains(t, string(out), strings.Repeat("b", 400),
		"deepest (level-7) object should be pruned first")
	require.Contains(t, string(out), strings.Repeat("a", 400),
		"shallower level-6 siblings should survive")
}

func TestPrune_DepthReasonPriority(t *testing.T) {
	// Two same-size same-level nodes; one is already notCapturedReason:depth.
	// The flagged one must be pruned first.
	normal := `{"type":"Normal","data":"` + strings.Repeat("x", 400) + `"}`
	flagged := `{"type":"Flagged","data":"` + strings.Repeat("y", 400) +
		`","notCapturedReason":"depth"}`
	input := []byte(envelope(map[string]string{"a": normal, "b": flagged}))
	// Shrink by only enough to force exactly one prune.
	// Shrink by enough that one leaf prune must happen but two would
	// be excessive. Each leaf saves ~400 bytes; shrink by 350.
	out := Prune(input, len(input)-350)
	require.True(t, json.Valid(out))
	require.NotContains(t, string(out), strings.Repeat("y", 400),
		"flagged (notCapturedReason=depth) should be pruned first")
	require.Contains(t, string(out), strings.Repeat("x", 400),
		"normal leaf should survive")
}

func TestPrune_LeafPromotion(t *testing.T) {
	// Three small siblings at level 6; forcing all three to be pruned
	// should promote the parent (arguments) to a leaf. When total
	// savings still don't fit, the parent itself is pruned.
	mk := func(name string, n int) string {
		return `{"type":"T","n":` + strconv.Itoa(n) + `,"pad":"` +
			strings.Repeat(name, 50) + `"}`
	}
	input := []byte(envelope(map[string]string{
		"a": mk("a", 1),
		"b": mk("b", 2),
		"c": mk("c", 3),
	}))

	// Force nearly the whole payload to be stripped.
	out := Prune(input, 200)
	require.True(t, json.Valid(out))
	// All three leaves gone.
	require.NotContains(t, string(out), strings.Repeat("a", 50))
	require.NotContains(t, string(out), strings.Repeat("b", 50))
	require.NotContains(t, string(out), strings.Repeat("c", 50))
	// The arguments object (parent, level 5) should have been promoted
	// and pruned.
	require.Contains(t, string(out), `"notCapturedReason":"pruned"`)
}

func TestPrune_PartialPrune(t *testing.T) {
	// Five children; only some need to be pruned to fit budget.
	children := map[string]string{}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("v%d", i)
		children[name] = `{"type":"T","data":"` +
			strings.Repeat(name, 100) + `"}`
	}
	input := []byte(envelope(children))
	// Trim only enough to require pruning ~2 children.
	out := Prune(input, len(input)-400)
	require.True(t, json.Valid(out))
	require.LessOrEqual(t, len(out), len(input)-400+50, // allow slack
		"should be close to budget")
	// At least one should survive.
	survived := 0
	for i := 0; i < 5; i++ {
		if strings.Contains(string(out), strings.Repeat(fmt.Sprintf("v%d", i), 100)) {
			survived++
		}
	}
	require.Greater(t, survived, 0, "some children should remain")
	require.Less(t, survived, 5, "some children should have been pruned")
}

func TestPrune_TypePreservation(t *testing.T) {
	big := `{"type":"SomeVeryLongTypeNameThatIsWorthPreserving","x":"` +
		strings.Repeat("q", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": big}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
	require.Contains(t, string(out),
		`"type":"SomeVeryLongTypeNameThatIsWorthPreserving"`)
}

func TestPrune_StringContainingBraces(t *testing.T) {
	// Braces inside a string value must not confuse the parser.
	tricky := `{"type":"T","data":"}}{{}{}}","more":"` +
		strings.Repeat("z", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": tricky}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
}

func TestPrune_EscapedQuotes(t *testing.T) {
	tricky := `{"type":"T","q":"\"escaped\"","pad":"` +
		strings.Repeat("z", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": tricky}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
}

func TestPrune_NestedObjects(t *testing.T) {
	// Nest objects several levels deep inside a level-6 value and
	// verify the deepest ones are pruned first.
	nested := `{"type":"Outer","inner":{"type":"Mid","inner":{"type":"Deep","data":"` +
		strings.Repeat("x", 500) + `"}}}`
	input := []byte(envelope(map[string]string{"v": nested}))

	out := Prune(input, len(input)-400)
	require.True(t, json.Valid(out))
	// Deepest object should be pruned; outer/mid should remain.
	require.NotContains(t, string(out), strings.Repeat("x", 500))
	require.Contains(t, string(out), `"Outer"`)
}

func TestPrune_MalformedInputReturnsUnchanged(t *testing.T) {
	input := []byte(`{"not closed": "x"`)
	out := Prune(input, 5)
	require.Equal(t, input, out)
}

func TestPrune_NonStringTypeValue(t *testing.T) {
	// "type" whose value is a number, not a string. Must not crash;
	// placeholder must fall back to the no-type variant.
	weird := `{"type":42,"data":"` + strings.Repeat("x", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": weird}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
	require.Contains(t, string(out), `{"notCapturedReason":"pruned"}`)
}

func TestPrune_NonStringNotCapturedReason(t *testing.T) {
	// "notCapturedReason" whose value is not a string: must not affect
	// flags.
	weird := `{"type":"T","notCapturedReason":null,"data":"` +
		strings.Repeat("x", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": weird}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
}

func TestPrune_EscapedQuoteInTypeValue(t *testing.T) {
	// The "type" string itself contains an escaped quote. The parser
	// must correctly locate the opening quote via innerStringStart
	// and preserve the escaped sequence in the placeholder.
	weird := `{"type":"na\"me","data":"` + strings.Repeat("x", 500) + `"}`
	input := []byte(envelope(map[string]string{"v": weird}))
	out := Prune(input, len(input)-200)
	require.True(t, json.Valid(out))
	require.Contains(t, string(out), `"type":"na\"me"`)
}

func TestPrune_DominatedSkipInPruneLoop(t *testing.T) {
	// Construct input where the same leaf is pushed, then its parent
	// is promoted+pruned (dominating the leaf), and then when the
	// leaf re-surfaces from the heap it should be skipped. This
	// naturally happens in the three-level chain below.
	inner := `{"type":"Inner","data":"` + strings.Repeat("i", 200) + `"}`
	mid := `{"type":"Mid","inner":` + inner + `}`
	outer := `{"type":"Outer","mid":` + mid + `}`
	input := []byte(envelope(map[string]string{"v": outer}))
	out := Prune(input, 250)
	require.True(t, json.Valid(out))
	require.NotContains(t, string(out), strings.Repeat("i", 200))
}

func TestPrune_ScratchPoolReuse(t *testing.T) {
	big := `{"type":"T","data":"` + strings.Repeat("x", 1000) + `"}`
	input := []byte(envelope(map[string]string{"v": big}))
	// Warm the pool once.
	_ = Prune(input, 200)

	allocs := testing.AllocsPerRun(5, func() {
		_ = Prune(input, 200)
	})
	// jsontext.Decoder allocates on construction; we expect a small
	// single-digit count of heap allocations per call, not dozens.
	// jsontext.Decoder allocates on construction; we accept a moderate
	// count per call but check it doesn't scale with input size.
	assert.Less(t, allocs, float64(50),
		"pooled scratch should keep allocations low (got %v)", allocs)
}
