// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

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

// CanonicalizeGenerics replaces each bracket-enclosed type parameter list with
// "[...]". For example, "pkg.T[go.shape.int].M" becomes "pkg.T[...].M".
// Handles nested brackets correctly.
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
	buf := make([]byte, 0, len(name))
	pos := 0
	for pos < len(name) {
		if name[pos] == '[' {
			end := MatchBracket(name, pos)
			if end == -1 {
				// Unmatched bracket — copy rest verbatim.
				buf = append(buf, name[pos:]...)
				return string(buf)
			}
			buf = append(buf, "[...]"...)
			pos = end + 1
		} else {
			buf = append(buf, name[pos])
			pos++
		}
	}
	return string(buf)
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
