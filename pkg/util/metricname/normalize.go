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

// survivesUnchanged reports whether byte c is emitted as itself, given that prev
// is the byte immediately before it in the region that has survived so far, and
// therefore also the last byte emitted.
//
// This is the rewrite rules read as a predicate; appendNormalizedFrom applies the
// same rules as a transformation. Keeping them adjacent is the point of merging
// the old separate isNormalized into this file: previously the rules were spelled
// out twice in full and a fuzz target had to prove the two agreed.
func survivesUnchanged(c, prev byte) bool {
	switch {
	case isAlphaNum(c):
		return true
	case c == '.':
		// A period overwrites a preceding underscore rather than being appended.
		return prev != '_'
	case c == '_':
		// An underscore after a period or another underscore is dropped.
		return prev != '.' && prev != '_'
	default:
		// Anything else becomes an underscore.
		return false
	}
}

// normalizedPrefix scans name once and reports how normalisation would treat it,
// without needing a buffer.
//
// keep is the number of leading bytes that normalisation emits verbatim, so a
// caller can copy them in bulk and resume from there rather than rebuilding the
// whole name a byte at a time. identical is true when that covers the whole name,
// i.e. the intake would store it unchanged and no rewrite is needed at all. ok is
// false when the intake would reject the name outright.
//
// Deviation is detected from the preceding byte alone, never by looking ahead,
// because in the surviving region the last emitted byte is always name[i-1].
//
// The subtlety, and the thing worth reviewing closely: an emitted underscore is
// not final. A later period overwrites it and a trailing one is stripped, and
// neither is visible at the point the underscore is scanned. `a___..b` normalises
// to `a..b`, so even the underscore at index 1 -- which survives its own
// inspection, because it sits between two alphanumerics -- is ultimately
// overwritten by the period at index 4. keep therefore never ends in an
// underscore. Asserted by TestNormalizedPrefixIsCopyable and by the prefix check
// inside TestNormalizationMatchesReferenceExhaustive.
func normalizedPrefix(name string) (keep int, identical bool, ok bool) {
	if len(name) == 0 || len(name) > maxLength {
		return 0, false, false
	}

	// A normalized name starts with an ASCII letter, because everything before
	// the first one is stripped. Anything else deviates at the very first byte,
	// and only then do we need to know whether a letter exists at all.
	if !isAlpha(name[0]) {
		if _, hasAlpha := firstAlpha(name); !hasAlpha {
			return 0, false, false
		}
		return 0, false, true
	}

	i := 1
	for ; i < len(name); i++ {
		if !survivesUnchanged(name[i], name[i-1]) {
			break
		}
	}

	if i == len(name) && name[i-1] != '_' {
		return len(name), true, true
	}

	// Back off any trailing underscore, which a later period would overwrite or
	// the end of the name would strip. name[0] is a letter so this cannot reach
	// zero, and the surviving region holds no run of underscores so it removes
	// at most one.
	for i > 0 && name[i-1] == '_' {
		i--
	}
	return i, false, true
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
// A normalized name is never longer than its input, and normalizedPrefix rejects
// anything longer than maxLength, so a dst with maxLength spare capacity is
// enough for append never to reallocate. That is what lets Matcher.Test normalize
// into a stack buffer, and is why this appends rather than returning a string:
// there is no production caller that wants the allocation.
//
// keep must come from normalizedPrefix for this same name. Those bytes are copied
// in bulk instead of being re-examined, which is the point of scanning once.
// keep == 0 means nothing survived, so the run before the first letter is skipped
// here instead.
func appendNormalizedFrom(dst []byte, name string, keep int) []byte {
	i := keep
	if keep == 0 {
		// firstAlpha cannot fail: normalizedPrefix already established that a
		// letter exists.
		i, _ = firstAlpha(name)
	} else {
		dst = append(dst, name[:keep]...)
	}

	// dst is now non-empty, or i points at a letter that the first iteration
	// appends, so the lookbacks below never read past the start.
	for ; i < len(name); i++ {
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

	return dst
}
