// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagname normalizes metric tags, and extracts their keys, in the name
// space the Datadog metrics intake stores them in.
//
// The Agent submits tags verbatim; the intake rewrites them on ingest. Any
// Agent-side decision that has to agree with what the intake stores (for
// example matching a tag filter list) must therefore compare normalized tags
// rather than the raw tags seen on the wire.
//
// The behavior is specified in SPEC.md, next to this file, which is normative.
// The test vectors in testdata/vectors.txt pin it down. Keep all three in sync.
package tagname

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLength is the output length, in bytes, past which no further code point
// is appended. See SPEC.md §4.
//
// The check happens before each code point is appended, so the output can
// exceed MaxLength by up to utf8.UTFMax-1 bytes. That is the intake's
// behavior, which this package reproduces. See MaxOutputLength.
const MaxLength = 200

// MaxOutputLength is the maximum length, in bytes, of a normalized tag or key.
const MaxOutputLength = MaxLength + utf8.UTFMax - 1

// maxInputOffset is the input byte offset past which no further code point is
// read. See SPEC.md §4.
const maxInputOffset = 2 * MaxLength

// Normalize returns tag as the intake stores it. See SPEC.md §4.
//
// An empty result means the intake drops the tag.
func Normalize(tag string) string {
	if isNormalizedASCII(tag) {
		return tag
	}
	return string(scan(make([]byte, 0, min(len(tag), MaxOutputLength)), tag, false))
}

// NormalizeAppend appends tag as the intake stores it to dst, and returns the
// extended slice. At most MaxOutputLength bytes are appended.
func NormalizeAppend(dst []byte, tag string) []byte {
	if isNormalizedASCII(tag) {
		return append(dst, tag...)
	}
	return scan(dst, tag, false)
}

// Key returns the key of tag as the intake stores it: the part of the
// normalized tag before its first ':', or the whole normalized tag if it has
// none. See SPEC.md §5.
//
// It does not allocate when the key can be read straight off tag, which is the
// case for the overwhelmingly common already-normalized ASCII tag.
func Key(tag string) string {
	if key, ok := fastKey(tag); ok {
		return key
	}
	return string(scan(make([]byte, 0, min(len(tag), MaxOutputLength)), tag, true))
}

// KeyAppend appends the key of tag as the intake stores it to dst, and returns
// the extended slice. At most MaxOutputLength bytes are appended, so a dst with
// that much spare capacity is never reallocated.
func KeyAppend(dst []byte, tag string) []byte {
	if key, ok := fastKey(tag); ok {
		return append(dst, key...)
	}
	return scan(dst, tag, true)
}

// RuleKey normalizes a tag key taken from a rule, such as a tag filter list
// entry, into the key the intake stores for tags with that key. See SPEC.md §6.
//
// The bool is false when no stored tag can have this key, which is when it
// contains a ':'. Such rules can never match.
//
// The empty key is a valid result: the intake stores tags such as `:foo` and
// `123:foo` with an empty key.
//
// RuleKey is the identity on keys the intake stores, so rules authored against
// stored keys are unaffected.
func RuleKey(key string) (string, bool) {
	if strings.IndexByte(key, ':') >= 0 {
		return "", false
	}
	return Key(key + ":"), true
}

// scan is the normalization state machine of SPEC.md §4.
//
// With stopAtColon, it returns as soon as a ':' would be emitted, without
// emitting it and without the final underscore strip, which yields the key of
// SPEC.md §5.
func scan(dst []byte, tag string, stopAtColon bool) []byte {
	base := len(dst)
	buf := dst
	lastWasUnderscore := false

	for i, c := range tag {
		n := len(buf) - base
		if n >= MaxLength || i > maxInputOffset {
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
			// Nothing but a letter or ':' can start a tag: skip.
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

// isNormalizedASCII reports whether tag is an ASCII tag the intake stores
// unchanged. It is a sufficient condition only: a non-ASCII tag can be
// normalized too, and is then handled by scan. See SPEC.md §7.
func isNormalizedASCII(tag string) bool {
	if len(tag) == 0 {
		return true
	}
	if len(tag) > MaxLength || !validASCIIStartChar[tag[0]] {
		return false
	}
	last := len(tag) - 1
	for i := 1; i < last; i++ {
		if c := tag[i]; !validASCIITagChar[c] && (c != '_' || tag[i-1] == '_') {
			return false
		}
	}
	return validASCIITagChar[tag[last]]
}

// fastKey returns the key of tag when it can be read straight off an ASCII
// prefix of tag that the intake stores unchanged, reporting false otherwise.
// Only the part of tag up to its first ':' is ever read. See SPEC.md §7.
func fastKey(tag string) (string, bool) {
	if len(tag) == 0 || tag[0] == ':' {
		return "", true
	}
	if !validASCIIStartChar[tag[0]] {
		return "", false
	}
	for i := 1; i < len(tag); i++ {
		switch c := tag[i]; {
		case c == ':':
			// The ':' is only reached while fewer than MaxLength bytes have
			// been emitted, which here is exactly i.
			return tag[:i], i < MaxLength
		case validASCIITagChar[c]:
		case c == '_' && tag[i-1] != '_':
		default:
			return "", false
		}
	}
	// No ':': the whole tag is the key, and is subject to truncation and to
	// the trailing underscore strip.
	return tag, len(tag) <= MaxLength && tag[len(tag)-1] != '_'
}

var validASCIIStartChar = [256]bool{
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true, 'h': true,
	'i': true, 'j': true, 'k': true, 'l': true, 'm': true, 'n': true, 'o': true, 'p': true,
	'q': true, 'r': true, 's': true, 't': true, 'u': true, 'v': true, 'w': true, 'x': true,
	'y': true, 'z': true, ':': true,
}

// validASCIITagChar holds the ASCII bytes kept verbatim anywhere but at the
// start of a tag. '_' is deliberately absent: it is only kept verbatim when it
// does not follow another '_', and not at the end.
var validASCIITagChar = [256]bool{
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true, 'h': true,
	'i': true, 'j': true, 'k': true, 'l': true, 'm': true, 'n': true, 'o': true, 'p': true,
	'q': true, 'r': true, 's': true, 't': true, 'u': true, 'v': true, 'w': true, 'x': true,
	'y': true, 'z': true, '0': true, '1': true, '2': true, '3': true, '4': true, '5': true,
	'6': true, '7': true, '8': true, '9': true, ':': true, '.': true, '/': true, '-': true,
}
