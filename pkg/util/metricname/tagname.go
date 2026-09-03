// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"unicode"
	"unicode/utf8"
)

// MaxTagLength is the maximum length of a tag in bytes.
//
// Unlike a metric name, a tag over this length is truncated by the intake rather
// than rejected. Mirrors `model.MaxTagLength` in dd-go.
//
// Note that the intake applies this limit to the whole `name:value` tag, so a
// tag name is never longer than this either. Tags submitted with the long tag
// option get a much larger budget (`model.MaxLongTagLength`, 5 KiB), which is
// deliberately not mirrored here: it only ever lets a *longer* name through, and
// a name that long cannot be typed into a filter list by hand anyway.
const MaxTagLength = 200

// MaxNormalizedTagLength is the maximum length in bytes of a normalized tag
// name, and therefore the buffer size NormalizeTagNameAppend needs to never
// reallocate.
//
// It exceeds MaxTagLength because the length is checked before appending a code
// point rather than after, so the last one can straddle the limit. dd-go has the
// same overshoot, documented as a bug on `model.MaxTagLength`; matching it
// matters, because the intake stores the overshooting name.
const MaxNormalizedTagLength = MaxTagLength + utf8.UTFMax - 1

// isLowerAlpha reports whether b is a lowercase ASCII letter.
func isLowerAlpha(b byte) bool {
	return b >= 'a' && b <= 'z'
}

// isDigit reports whether b is an ASCII digit.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// IsNormalizedASCIITagName reports whether name is an all-ASCII tag name that the
// intake would store unchanged. See NormalizeTagNameAppend for the trailing
// underscore, which is part of a normalized name.
//
// Like IsNormalized it exists to let callers skip the rewrite for the
// overwhelmingly common case, and it performs a single pass and never allocates.
//
// Unlike IsNormalized the predicate is one-sided, mirroring
// `model.IsNormalizedASCIITag` in dd-go: true guarantees that normalizing name
// is the identity, but false does not guarantee the opposite. A name containing
// non-ASCII bytes is always reported as not normalized, even when it is already
// a fixed point (`café` is stored as `café`), because deciding that needs the
// Unicode tables that the fast path exists to avoid. Such a name simply takes
// the slow path, which returns it unchanged.
//
// The one-sidedness is why callers must use the result only to skip work, never
// to decide that a name is unstorable.
func IsNormalizedASCIITagName(name string) bool {
	if len(name) == 0 || len(name) > MaxTagLength {
		return false
	}

	// A normalized tag name always starts with a lowercase ASCII letter (or a
	// non-ASCII letter, which this fast path does not accept): everything before
	// the first letter is stripped, and letters are lowercased.
	if !isLowerAlpha(name[0]) {
		return false
	}

	for i := 1; i < len(name); i++ {
		switch c := name[i]; {
		case isLowerAlpha(c) || isDigit(c) || c == '.' || c == '/' || c == '-':
			// Kept verbatim.
		case c == '_':
			// Runs of underscores collapse to one. Note that, unlike in a metric
			// name, an underscore next to a period or at the end of the name is
			// left alone.
			if name[i-1] == '_' {
				return false
			}
		default:
			// Anything else, including a colon and any non-ASCII byte, means the
			// name is either rewritten or not a tag name at all.
			return false
		}
	}

	return true
}

// NormalizeTagNameAppend appends the tag name as the Datadog intake will store it
// to dst, and returns the extended slice.
//
// The bool is false when the name normalizes to nothing, which happens when it is
// empty or contains no letter at all: the intake drops such a tag. In that case
// dst is returned unmodified.
//
// name must be a tag name, i.e. the part of a tag before the first colon. A colon
// is not expected and, like any other invalid byte, becomes an underscore --
// callers holding a whole `name:value` tag must split it first.
//
// The rules, applied per code point:
//
//  1. Everything before the first letter is discarded.
//  2. Letters are lowercased.
//  3. Digits and `.`, `/` and `-` are kept verbatim.
//  4. Every other code point becomes an underscore, and an underscore is not
//     emitted directly after another one. Note that this applies to literal
//     underscores in the input too, so `a__b` becomes `a_b`.
//  5. The name is truncated to MaxTagLength.
//
// A trailing underscore is deliberately *kept*: the intake strips one only from
// the very end of the whole tag, so the name in `my_tag_:value` is stored as
// `my_tag_`. A caller holding a tag that carries no value, where the name does end
// the tag, has to drop that underscore itself.
//
// These are *not* the metric name rules in NormalizeAppend: tag names are
// lowercased, keep `-` and `/`, keep an underscore next to a period, are
// Unicode-aware rather than byte-wise, and are truncated instead of rejected when
// too long. Use the right one for the name at hand.
//
// Normalizing is idempotent, and the output of an all-ASCII input satisfies
// IsNormalizedASCIITagName.
//
// A dst with MaxNormalizedTagLength spare capacity is enough for append never to
// reallocate, which is what lets callers normalize into a stack buffer.
//
// This is a port of `NormalizeTag` in dd-go (`model/tags.go`), restricted to the
// tag name. Keep the two in sync: a divergence here silently changes which tags
// get stripped. Note that `NormalizeTagArbTagValue`, the path taken only when a
// payload opts in with `allow_arbitrary_tags`, normalizes the name slightly
// differently: it strips a trailing underscore from the name even when a value
// follows. This follows the default path.
func NormalizeTagNameAppend(dst []byte, name string) ([]byte, bool) {
	start := len(dst)
	lastWasUnderscore := false

	for i, c := range name {
		if len(dst)-start >= MaxTagLength {
			break
		}
		// Bound the work done on a very long name that is mostly garbage: past
		// this point it cannot contribute to the truncated result anyway. dd-go
		// bounds it the same way, on the byte offset rather than the count of
		// code points read.
		if i > 2*MaxTagLength {
			break
		}

		switch {
		// Fast path for the ASCII alphabet.
		case c >= 'a' && c <= 'z':
			dst = append(dst, byte(c))
			lastWasUnderscore = false
		case c >= 'A' && c <= 'Z':
			dst = append(dst, byte(c)+('a'-'A'))
			lastWasUnderscore = false
		case unicode.IsLetter(c):
			dst = utf8.AppendRune(dst, unicode.ToLower(c))
			lastWasUnderscore = false
		// Skip anything that cannot start the name. Reached only while nothing
		// has been emitted yet, so only until the first letter.
		case len(dst) == start:
		// Valid, but cannot start the name.
		case c == '.' || c == '/' || c == '-':
			dst = append(dst, byte(c))
			lastWasUnderscore = false
		case unicode.IsDigit(c):
			dst = utf8.AppendRune(dst, c)
			lastWasUnderscore = false
		// Convert anything else to an underscore, including a literal
		// underscore, but only one in a row.
		case !lastWasUnderscore:
			dst = append(dst, '_')
			lastWasUnderscore = true
		}
	}

	if len(dst) == start {
		return dst, false
	}

	return dst, true
}
