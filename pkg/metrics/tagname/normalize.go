// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagname extracts the key of a metric tag, and normalizes rule keys,
// in the form the Datadog metrics intake stores them in.
//
// The Agent submits tags verbatim and the intake normalizes them on ingest, so
// a decision that has to agree with what the intake stores, such as matching a
// tag filter list, must compare normalized keys. SPEC.md, next to this file,
// defines the behavior, and the vectors in testdata/ pin it down.
package tagname

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/twmb/murmur3"
)

// maxLength is the output length, in bytes, at or after which no further code
// point is appended (SPEC.md §4). The check runs before appending, so the last
// code point can take the output up to maxOutputLength.
const maxLength = 200

const maxOutputLength = maxLength + utf8.UTFMax - 1

// maxInputOffset is the largest input byte offset still read (SPEC.md §4).
const maxInputOffset = 2 * maxLength

// KeyHash returns the Murmur3 hash of the key the intake stores for tag
// (SPEC.md §5), without allocating.
func KeyHash(tag string) uint64 {
	if key, ok := fastKey(tag); ok {
		return murmur3.StringSum64(key)
	}
	var buf [maxOutputLength]byte
	return murmur3.Sum64(scan(buf[:0], tag, tagKey))
}

// RuleKey normalizes a tag key from a rule, such as a tag filter list entry,
// into the key the intake stores for tags with that key: R(k) of SPEC.md §6.
// The bool is false when the key contains ':', which no stored key can.
func RuleKey(key string) (string, bool) {
	if strings.IndexByte(key, ':') >= 0 {
		return "", false
	}
	var buf [maxOutputLength]byte
	return string(scan(buf[:0], key, ruleKey)), true
}

type scanMode uint8

const (
	tagKey   scanMode = iota // Stop where the first ':' would be appended.
	ruleKey                  // Also treat the end of input as a synthetic ':'.
	wholeTag                 // Normalize the entire tag (tests only).
)

// scan runs the state machine of SPEC.md §4 and appends its output to dst.
func scan(dst []byte, tag string, mode scanMode) []byte {
	base := len(dst)
	buf := dst
	lastWasUnderscore := false

	for i, c := range tag {
		n := len(buf) - base
		if n >= maxLength || i > maxInputOffset {
			break
		}

		switch {
		case c >= 'a' && c <= 'z':
			buf = append(buf, byte(c))
			lastWasUnderscore = false
		case c >= 'A' && c <= 'Z':
			buf = append(buf, byte(c+('a'-'A')))
			lastWasUnderscore = false
		case c == ':':
			if mode != wholeTag {
				return buf
			}
			buf = append(buf, ':')
			lastWasUnderscore = false
		case unicode.IsLetter(c):
			buf = utf8.AppendRune(buf, unicode.ToLower(c))
			lastWasUnderscore = false
		case n == 0:
			// Skip: only a letter or ':' can start a tag.
		case c == '.' || c == '/' || c == '-':
			buf = append(buf, byte(c))
			lastWasUnderscore = false
		case unicode.IsDigit(c):
			buf = utf8.AppendRune(buf, c)
			lastWasUnderscore = false
		case !lastWasUnderscore:
			buf = append(buf, '_')
			lastWasUnderscore = true
		}
	}

	// A rule key gets a synthetic ':' at offset len(tag). If the intake
	// would reach and append it, keep any trailing '_' as for `foo_:x`.
	if mode == ruleKey && len(tag) <= maxInputOffset && len(buf)-base < maxLength {
		return buf
	}
	if lastWasUnderscore {
		buf = buf[:len(buf)-1]
	}
	return buf
}

// fastKey is the fast path of SPEC.md §7: it returns the key when it can be
// read straight off tag, reporting false otherwise.
func fastKey(tag string) (string, bool) {
	if len(tag) == 0 || tag[0] == ':' {
		return "", true
	}
	if tag[0] < 'a' || tag[0] > 'z' {
		return "", false
	}
	for i := 1; i < len(tag); i++ {
		switch c := tag[i]; {
		case i >= maxLength:
			// A ':' this late is never emitted, and a key this long is
			// truncated: only the state machine handles either.
			return "", false
		case c == ':':
			return tag[:i], true
		case validASCIIKeyChar[c]:
		case c == '_' && tag[i-1] != '_':
		default:
			return "", false
		}
	}
	return tag, len(tag) <= maxLength && tag[len(tag)-1] != '_'
}

// validASCIIKeyChar holds the bytes kept verbatim after the first byte of a
// key. '_' is handled separately, since it is only kept when not repeated.
var validASCIIKeyChar = [256]bool{
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true, 'h': true,
	'i': true, 'j': true, 'k': true, 'l': true, 'm': true, 'n': true, 'o': true, 'p': true,
	'q': true, 'r': true, 's': true, 't': true, 'u': true, 'v': true, 'w': true, 'x': true,
	'y': true, 'z': true, '0': true, '1': true, '2': true, '3': true, '4': true, '5': true,
	'6': true, '7': true, '8': true, '9': true, '.': true, '/': true, '-': true,
}
