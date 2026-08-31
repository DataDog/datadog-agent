// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricname matches metric names against a list, in the name space the
// Datadog metrics intake stores them in.
//
// The Agent submits metric names verbatim; the backend rewrites them on ingest.
// Any Agent-side decision that has to agree with what the backend stores (for
// example matching a metric filter list) must therefore compare normalized
// names rather than the raw names seen on the wire.
//
// Matcher is the only entry point. The normalization itself is deliberately not
// exported: the sole production caller is Matcher.Test, and it wants to
// normalize into a stack buffer rather than pay for a returned string. Export an
// append-style helper if another package ever needs it, rather than one that
// allocates.
//
// This is a faithful port of `NormMetricNameParse` / `ValidateMetricName` in
// dd-go (`model/metric.go`). Keep the two in sync: a divergence here silently
// changes which metrics get filtered.
package metricname

// maxLength is the maximum allowed length of a metric name in bytes.
//
// Names longer than this are rejected outright by the intake (they are not
// truncated). Mirrors `model.MaxMetricLen` in dd-go.
//
// Note that the public documentation states a 200 character limit; 350 is what
// the intake actually enforces, so it is what we mirror here.
const maxLength = 350

// isAlpha reports whether b is an ASCII letter.
//
// The intake works on bytes, not runes, so non-ASCII letters are deliberately
// not accepted here.
func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isAlphaNum reports whether b is an ASCII letter or digit.
func isAlphaNum(b byte) bool {
	return isAlpha(b) || (b >= '0' && b <= '9')
}

// firstAlpha returns the index of the first ASCII letter in name, reporting
// false if name is empty, longer than maxLength, or contains no ASCII letter.
// Such names are dropped by the intake.
func firstAlpha(name string) (int, bool) {
	if len(name) == 0 || len(name) > maxLength {
		return 0, false
	}

	for i := 0; i < len(name); i++ {
		if isAlpha(name[i]) {
			return i, true
		}
	}

	return 0, false
}

// isNormalized reports whether name is a name the intake would store unchanged,
// i.e. whether normalizing it would be the identity.
//
// It performs a single pass and never allocates, which is what lets filter list
// matching skip the rewrite entirely for the overwhelmingly common case of an
// already-normalized name. See Matcher.Test.
//
// The predicate is exact: isNormalized(s) is true if and only if normalizing s
// yields s unchanged. TestIsNormalizedMatchesNormalize and the fuzz target
// beside it assert that equivalence.
func isNormalized(name string) bool {
	if len(name) == 0 || len(name) > maxLength {
		return false
	}

	// A normalized name always starts with an ASCII letter, because everything
	// before the first one is stripped.
	if !isAlpha(name[0]) {
		return false
	}

	for i := 1; i < len(name); i++ {
		switch c := name[i]; {
		case isAlphaNum(c) || c == '.':
			// Kept verbatim. Note that runs of periods and a trailing period
			// are both legal in a normalized name.
		case c == '_':
			// An underscore is only ever emitted between two alphanumerics: it
			// is not emitted after a period or another underscore, a following
			// period overwrites it, and a trailing one is stripped.
			if !isAlphaNum(name[i-1]) {
				return false
			}
			if i == len(name)-1 || !isAlphaNum(name[i+1]) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// normalizeAppend appends the metric name as the Datadog intake will store it to
// dst, and returns the extended slice.
//
// The bool is false when the intake would reject the name outright rather than
// rewrite it, which happens when the name is empty, longer than maxLength bytes,
// or contains no ASCII letter. In that case dst is returned unmodified and
// callers should treat the name as unstorable.
//
// The rules, applied byte-wise:
//
//  1. Everything before the first ASCII letter is discarded.
//  2. ASCII alphanumerics are kept verbatim; case is preserved.
//  3. A period is kept, but a period following an underscore replaces it.
//  4. Every other byte becomes an underscore, and an underscore is not emitted
//     directly after a period or another underscore. Note that this applies to
//     literal underscores in the input too, so `a._b` becomes `a.b`.
//  5. A trailing underscore is stripped.
//
// Because step 4 works on bytes, each byte of a multi-byte UTF-8 sequence is
// treated separately and the sequence collapses to a single underscore.
//
// Normalizing is idempotent: the output always satisfies isNormalized.
//
// A normalized name is never longer than its input, and firstAlpha rejects
// anything longer than maxLength, so a dst with maxLength spare capacity is
// enough for append never to reallocate. That is what lets Matcher.Test
// normalize into a stack buffer, and is why this appends rather than returning a
// string: there is no production caller that wants the allocation.
func normalizeAppend(dst []byte, name string) ([]byte, bool) {
	start, ok := firstAlpha(name)
	if !ok {
		return dst, false
	}

	// The first iteration always appends, because name[start] is a letter, so
	// the lookbacks below never read past the end of what this call wrote.
	for i := start; i < len(name); i++ {
		switch c := name[i]; {
		case isAlphaNum(c):
			dst = append(dst, c)
		case c == '.':
			if dst[len(dst)-1] == '_' {
				// Overwrite an underscore that happens just before a period.
				dst[len(dst)-1] = '.'
			} else {
				dst = append(dst, '.')
			}
		default:
			// No double underscores and no underscore after a period.
			if last := dst[len(dst)-1]; last != '.' && last != '_' {
				dst = append(dst, '_')
			}
		}
	}

	if dst[len(dst)-1] == '_' {
		dst = dst[:len(dst)-1]
	}

	return dst, true
}
