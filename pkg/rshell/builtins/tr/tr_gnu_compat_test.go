// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// GNU compatibility tests for the tr builtin.
//
// Expected outputs were captured from GNU coreutils tr 9.6
// and are embedded as string literals so the tests run without any GNU
// tooling present on CI.

package tr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatTranslateBasic — basic character translation.
//
// GNU command: echo -n 'abcd' | gtr abcd '[]*]'
// Expected: "]]]]"
func TestGNUCompatTranslateBasic(t *testing.T) {
	stdout, _, code := trRun(t, "abcd", "abcd '[]*]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "]]]]", stdout)
}

// TestGNUCompatRangeTranslation — translate a-z to A-Z.
//
// GNU command: echo -n '!abcd!' | gtr a-z A-Z
// Expected: "!ABCD!"
func TestGNUCompatRangeTranslation(t *testing.T) {
	stdout, _, code := trRun(t, "!abcd!", "a-z A-Z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "!ABCD!", stdout)
}

// TestGNUCompatTruncate — -t truncates set1 to set2 length.
//
// GNU command: echo -n 'abcde' | gtr -t abcd xy
// Expected: "xycde"
func TestGNUCompatTruncate(t *testing.T) {
	stdout, _, code := trRun(t, "abcde", "-t abcd xy")
	assert.Equal(t, 0, code)
	assert.Equal(t, "xycde", stdout)
}

// TestGNUCompatSet1LongerBSD — default BSD behavior pads set2 with last char.
//
// GNU command: echo -n 'abcde' | gtr abcd xy
// Expected: "xyyye"
func TestGNUCompatSet1LongerBSD(t *testing.T) {
	stdout, _, code := trRun(t, "abcde", "abcd xy")
	assert.Equal(t, 0, code)
	assert.Equal(t, "xyyye", stdout)
}

// TestGNUCompatSqueeze — -s squeezes repeated characters.
//
// GNU command: echo -n 'aabbcc' | gtr -s '[a-z]'
// Expected: "abc"
func TestGNUCompatSqueeze(t *testing.T) {
	stdout, _, code := trRun(t, "aabbcc", "-s '[a-z]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc", stdout)
}

// TestGNUCompatDeleteRange — -d deletes matching bytes.
//
// GNU command: echo -n 'abc $code' | gtr -d a-z
// Expected: " $"
func TestGNUCompatDeleteRange(t *testing.T) {
	stdout, _, code := trRun(t, "abc $code", "-d a-z")
	assert.Equal(t, 0, code)
	assert.Equal(t, " $", stdout)
}

// TestGNUCompatDeleteAndSqueeze — -ds deletes set1 then squeezes set2.
//
// GNU command: echo -n 'aabbaa' | gtr -ds b a
// Expected: "a"
func TestGNUCompatDeleteAndSqueeze(t *testing.T) {
	stdout, _, code := trRun(t, "aabbaa", "-ds b a")
	assert.Equal(t, 0, code)
	assert.Equal(t, "a", stdout)
}

// TestGNUCompatClassLower — [:lower:] to [:upper:] case conversion.
//
// GNU command: echo -n 'abcxyzABCXYZ' | gtr '[:lower:]' '[:upper:]'
// Expected: "ABCXYZABCXYZ"
func TestGNUCompatClassLower(t *testing.T) {
	stdout, _, code := trRun(t, "abcxyzABCXYZ", "'[:lower:]' '[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCXYZABCXYZ", stdout)
}

// TestGNUCompatClassUpper — [:upper:] to [:lower:] case conversion.
//
// GNU command: echo -n 'abcxyzABCXYZ' | gtr '[:upper:]' '[:lower:]'
// Expected: "abcxyzabcxyz"
func TestGNUCompatClassUpper(t *testing.T) {
	stdout, _, code := trRun(t, "abcxyzABCXYZ", "'[:upper:]' '[:lower:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcxyzabcxyz", stdout)
}

// TestGNUCompatClassicWordSplit — classic word-per-line example.
//
// GNU command: echo -n 'The big black fox jumped over the fence.' | gtr -cs '[:alnum:]' '\n'
// Expected: "The\nbig\nblack\nfox\njumped\nover\nthe\nfence\n"
func TestGNUCompatClassicWordSplit(t *testing.T) {
	stdout, _, code := trRun(t, "The big black fox jumped over the fence.", `-cs '[:alnum:]' '\n'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "The\nbig\nblack\nfox\njumped\nover\nthe\nfence\n", stdout)
}

// TestGNUCompatDeleteXdigit — delete hex digit class.
//
// GNU command: echo -n 'w0x1y2z3456789acbdefABCDEFz' | gtr -d '[:xdigit:]'
// Expected: "wxyzz"
func TestGNUCompatDeleteXdigit(t *testing.T) {
	stdout, _, code := trRun(t, "w0x1y2z3456789acbdefABCDEFz", "-d '[:xdigit:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "wxyzz", stdout)
}

// TestGNUCompatEmptyInput — empty input produces empty output.
//
// GNU command: echo -n ” | gtr a b
// Expected: ""
func TestGNUCompatEmptyInput(t *testing.T) {
	stdout, _, code := trRun(t, "", "a b")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatRangeAToA — a-a range is accepted.
//
// GNU command: echo -n 'abc' | gtr a-a z
// Expected: "zbc"
func TestGNUCompatRangeAToA(t *testing.T) {
	stdout, _, code := trRun(t, "abc", "a-a z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "zbc", stdout)
}

// TestGNUCompatNullSet2 — empty set2 with non-empty set1 in translate mode.
//
// GNU command: echo -n ” | gtr a ”  → exit 1
// Expected: stderr contains "string2 must be non-empty"
func TestGNUCompatNullSet2(t *testing.T) {
	_, stderr, code := trRun(t, "", "a ''")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "string2 must be non-empty")
}

// TestGNUCompatFowler — operand that looks like a flag.
//
// GNU command: echo -n 'aha' | gtr ah -H
// Expected: "-H-"
func TestGNUCompatFowler(t *testing.T) {
	stdout, _, code := trRun(t, "aha", "ah -H")
	assert.Equal(t, 0, code)
	assert.Equal(t, "-H-", stdout)
}

// TestGNUCompatRepeatZero — [b*0] means fill to set1 length.
//
// GNU command: echo -n 'abcd' | gtr abc '[b*0]'
// Expected: "bbbd"
func TestGNUCompatRepeatZero(t *testing.T) {
	stdout, _, code := trRun(t, "abcd", "abc '[b*0]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbbd", stdout)
}

// TestGNUCompatComplementTranslate — -c translates complement of set1.
//
// GNU command: echo -n 'ab' | gtr -c a X
// Expected: "aX"
func TestGNUCompatComplementTranslate(t *testing.T) {
	stdout, _, code := trRun(t, "ab", "-c a X")
	assert.Equal(t, 0, code)
	assert.Equal(t, "aX", stdout)
}
