// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import "strings"

// GenericParams holds the text of generic type parameters from a symbol name.
type GenericParams struct {
	// Raw is the text between the outermost '[' and ']', not including the
	// brackets themselves.
	Raw string
	// Start is the byte offset of '[' in the raw symbol name.
	Start int
	// End is the byte offset of the matching ']' in the raw symbol name.
	End int
}

// CanonicalizeGenerics replaces each bracket-enclosed segment in a Go symbol
// name with "[...]". It is meant for symbol strings (DWARF, go.shape.*,
// etc.), not for pretty-printing or parsing Go types. The implementation does
// not distinguish generic type arguments from brackets that belong to slice,
// map, or array spellings in a type string, so feeding a bare type-like
// string (e.g. "[]int" or "map[string]int") produces odd-looking output
// such as "[...]int" or "map[...]int"; that is still correct for the
// bracket-stripping contract on arbitrary symbol text. For example,
// "pkg.T[go.shape.int].M" becomes "pkg.T[...].M". Pairs are found with
// bracket depth, so nested brackets form one segment and successive slice
// brackets like [][]T yield multiple segments.
func CanonicalizeGenerics(name string) string {
	// Fast path: no brackets at all.
	i := 0
	for i < len(name) {
		if name[i] == '[' {
			break
		}
		i++
	}
	if i == len(name) {
		return name
	}

	// Build result by copying non-bracket parts and replacing bracket contents.
	var b strings.Builder
	pos := 0
	for pos < len(name) {
		if name[pos] == '[' {
			end := MatchBracket(name, pos)
			if end == -1 {
				// Unmatched bracket — copy rest verbatim.
				b.WriteString(name[pos:])
				return b.String()
			}
			b.WriteString("[...]")
			pos = end + 1
		} else {
			b.WriteByte(name[pos])
			pos++
		}
	}
	return b.String()
}

// MatchBracket finds the matching ']' for the '[' at position start in s,
// handling nested brackets. Returns the index of the matching ']', or -1 if
// not found.
func MatchBracket(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
