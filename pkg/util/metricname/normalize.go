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
// This is a faithful port of `NormMetricNameParse` / `ValidateMetricName` in
// dd-go (`model/metric.go`). Keep the two in sync: a divergence here silently
// changes which metrics get filtered.
package metricname

// MaxLength is the maximum allowed length of a metric name in bytes.
//
// Names longer than this are rejected outright by the intake (they are not
// truncated). Mirrors `model.MaxMetricLen` in dd-go.
//
// Note that the public documentation states a 200 character limit; 350 is what
// the intake actually enforces, so it is what we mirror here.
const MaxLength = 350

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
// false if name is empty, longer than MaxLength, or contains no ASCII letter.
// Such names are dropped by the intake.
func firstAlpha(name string) (int, bool) {
	if len(name) == 0 || len(name) > MaxLength {
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
	if len(name) == 0 || len(name) > MaxLength {
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
// rewrite it, which happens when the name is empty, longer than MaxLength bytes,
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
// anything longer than MaxLength, so a dst with MaxLength spare capacity is
// enough for append never to reallocate. That is what lets Matcher.Test
// normalize into a stack buffer, and is why this appends rather than returning a
// string: no caller is forced to pay for the rewrite allocation.
func NormalizeAppend(dst []byte, name string) ([]byte, bool) {
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
			switch dst[len(dst)-1] {
			// overwrite underscores that happens before periods
			case '_':
				dst[len(dst)-1] = '.'
			default:
				dst = append(dst, '.')
			}
		default:
			// we skipped all non-alpha chars up front so we have seen at least one
			switch dst[len(dst)-1] {
			// no double underscores and no underscore after a period.
			case '.', '_':
			default:
				dst = append(dst, '_')
			}
		}
	}

	if dst[len(dst)-1] == '_' {
		dst = dst[:len(dst)-1]
	}

	return dst, true
}

// NormalizePrefixAppend appends the metric name prefix as the intake would store
// the start of the names it matches to dst, and returns the extended slice.
//
// A prefix is not a complete metric name: it is the beginning of the names it
// matches, so it keeps the boundary bytes that a name only keeps when an
// alphanumeric follows them. NormalizeAppend strips a trailing underscore,
// because no stored name ends with one, but doing that to a prefix widens it:
// `service_` would become `service` and start matching `service.requests` too,
// instead of only the `service_...` family the entry names. So the last byte is
// normalized as if an alphanumeric followed it, which is exactly the case in
// every name the prefix has to match.
//
// The byte following a prefix is what decides its boundary, and a prefix does not
// get to know it: the raw prefix `service_` is the `service_...` family in
// `service_requests`, but the `service.` family in `service_.requests` (which the
// intake stores as `service.requests`). The ambiguity is resolved towards the
// family the entry spells out, i.e. the one an alphanumeric continues; covering
// both would mean widening the prefix to `service`, which drops every
// `service`-something metric.
//
// The bool is false when no stored name can start with the prefix, which is when
// NormalizeAppend rejects it: a stored name never starts with a byte the intake
// strips, so `123.` matches nothing rather than everything. The empty prefix is
// the exception: it matches every name, and normalizes to itself.
//
// As with NormalizeAppend the output is never longer than the input, so a dst
// with MaxLength spare capacity is enough for append never to reallocate: the
// boundary underscore is only ever appended in place of an input byte that
// contributed nothing to the output.
func NormalizePrefixAppend(dst []byte, prefix string) ([]byte, bool) {
	// Nothing to normalize: the empty prefix matches every name.
	if prefix == "" {
		return dst, true
	}

	dst, ok := NormalizeAppend(dst, prefix)
	if !ok {
		return dst, false
	}

	// Restore the boundary NormalizeAppend dropped, if any. A byte that is
	// neither alphanumeric nor a period is emitted as an underscore when an
	// alphanumeric follows it, unless a period precedes it: a period is kept as
	// is, and absorbs the underscore that would come before it.
	if last := prefix[len(prefix)-1]; !isAlphaNum(last) && last != '.' && dst[len(dst)-1] != '.' {
		dst = append(dst, '_')
	}

	return dst, true
}
