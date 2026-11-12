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
)

// maxLength is the output length, in bytes, at or after which no further code
// point is appended (SPEC.md §4). The check runs before appending, so the last
// code point can take the output up to maxOutputLength.
const maxLength = 200

const maxOutputLength = maxLength + utf8.UTFMax - 1

// maxInputOffset is the largest input byte offset still read (SPEC.md §4).
const maxInputOffset = 2 * maxLength

// Key returns the key the intake stores for tag: K(t) of SPEC.md §5.
//
// It does not allocate for an already-normalized ASCII tag.
func Key(tag string) string {
	if key, ok := fastKey(tag); ok {
		return key
	}
	return string(scan(make([]byte, 0, min(len(tag), maxOutputLength)), tag, true))
}

// RuleKey normalizes a tag key from a rule, such as a tag filter list entry,
// into the key the intake stores for tags with that key: R(k) of SPEC.md §6.
// The bool is false when the key contains ':', which no stored key can.
func RuleKey(key string) (string, bool) {
	if strings.IndexByte(key, ':') >= 0 {
		return "", false
	}
	// Appending ':' normalizes key as the key of a tag with a value, so a
	// trailing '_' that is part of a stored key survives: `foo_:x` has the key
	// `foo_`, but the bare tag `foo_` has the key `foo`.
	return Key(key + ":"), true
}

// scan runs the state machine of SPEC.md §4 and appends its output to dst.
// With stopAtColon, it returns the key as soon as a ':' would be appended
// (SPEC.md §5). Only the tests use the whole-tag mode.
func scan(dst []byte, tag string, stopAtColon bool) []byte {
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
			if stopAtColon {
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
