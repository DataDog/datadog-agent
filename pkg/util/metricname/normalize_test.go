// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalize is the string-returning form of the single-pass normaliser, for
// tests that assert on the normalized name. Production code goes through
// normalizedPrefix + appendNormalizedFrom with a stack buffer instead, so this
// deliberately exists only in tests -- see Matcher.Test.
func normalize(name string) (string, bool) {
	keep, identical, ok := normalizedPrefix(name)
	if !ok {
		return name, false
	}
	if identical {
		return name, true
	}
	return string(appendNormalizedFrom(make([]byte, 0, maxLength), name, keep)), true
}

// isNormalized is the predicate the production code no longer needs as separate
// code: a name is normalized exactly when the single pass reports it unchanged.
func isNormalized(name string) bool {
	_, identical, ok := normalizedPrefix(name)
	return ok && identical
}

// normalizedNames mirrors the `testMetricNames` table in dd-go
// (`model/metric_test.go`) so a divergence between the two implementations
// shows up as a test failure here.
var normalizedNames = map[string]string{
	// bad metric names, need remapping
	"test*&(*._-_Metrictastic*(*)(  wut_who_doesthis??": "test.Metrictastic_wut_who_doesthis",
	"?does.this.work?":                        "does.this.work",
	"5-2 arsenal over spurs":                  "arsenal_over_spurs",
	"dd.crawler.amazon web services.run_time": "dd.crawler.amazon_web_services.run_time",

	// multiple metric names that normalize to the same thing
	"multiple-norm-1": "multiple_norm_1",
	"multiple_norm-1": "multiple_norm_1",

	// invalid characters are dropped rather than doubled up
	"a$.b":            "a.b",
	"a_.b":            "a.b",
	"__init__.metric": "init.metric",
	"a___..b":         "a..b",
	"a_.":             "a.",

	// an underscore is only ever kept between two alphanumerics, so literal
	// underscores next to a period or to each other are dropped
	"a._b": "a.b",
	"a__b": "a_b",
	"a_b":  "a_b",
	"a_":   "a",

	// already normalized, must be returned untouched
	"foo":                         "foo",
	"n_o_i_n_d_e_x.pct_aggr.1234": "n_o_i_n_d_e_x.pct_aggr.1234",

	// case is preserved, unlike tags
	"MyMetric.Count": "MyMetric.Count",

	// leading non-letters are stripped, not rejected
	"1app.requests": "app.requests",
	"...foo":        "foo",

	// runs of periods and a trailing period survive
	"foo...bar": "foo...bar",
	"foo.bar.":  "foo.bar.",

	// spaces and punctuation collapse to a single underscore
	"my metric  name":   "my_metric_name",
	"app-request-count": "app_request_count",
	"host.cpu%util":     "host.cpu_util",

	// non-ASCII is handled byte-wise and collapses away
	"café.requests": "caf.requests",
	"🍣.metric":      "metric",
}

// unstorableNames are names the intake rejects outright rather than rewriting.
var unstorableNames = []string{
	"",
	"_",
	"...",
	"123",
	"🍣",
	strings.Repeat("a", maxLength+1),
}

func TestNormalize(t *testing.T) {
	for input, expected := range normalizedNames {
		t.Run(input, func(t *testing.T) {
			actual, ok := normalize(input)
			require.True(t, ok)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	for input := range normalizedNames {
		t.Run(input, func(t *testing.T) {
			once, ok := normalize(input)
			require.True(t, ok)
			twice, ok := normalize(once)
			require.True(t, ok)
			assert.Equal(t, once, twice)
		})
	}
}

func TestNormalizeUnstorableNames(t *testing.T) {
	for _, input := range unstorableNames {
		t.Run(input, func(t *testing.T) {
			actual, ok := normalize(input)
			assert.False(t, ok, "expected %q to be rejected", input)
			assert.Equal(t, input, actual, "rejected names must be returned unchanged")
		})
	}
}

func TestNormalizeMaxLength(t *testing.T) {
	// A name of exactly maxLength bytes is accepted.
	atLimit := strings.Repeat("a", maxLength)
	actual, ok := normalize(atLimit)
	require.True(t, ok)
	assert.Equal(t, atLimit, actual)

	// One byte over is rejected, and is *not* truncated to fit.
	overLimit := atLimit + "a"
	actual, ok = normalize(overLimit)
	assert.False(t, ok)
	assert.Equal(t, overLimit, actual)

	// A name that would fit only after normalization is still rejected, because
	// the intake checks the length of the raw name.
	shrinks := strings.Repeat("a-", maxLength)
	_, ok = normalize(shrinks)
	assert.False(t, ok)
}

func TestIsNormalized(t *testing.T) {
	for input, expected := range normalizedNames {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, input == expected, isNormalized(input))
		})
	}

	for _, input := range unstorableNames {
		t.Run(input, func(t *testing.T) {
			assert.False(t, isNormalized(input))
		})
	}
}

// TestIsNormalizedMatchesNormalize asserts the property the fast path in
// Normalize relies on: isNormalized(s) is true exactly when normalize(s)
// returns s unchanged.
func TestIsNormalizedMatchesNormalize(t *testing.T) {
	inputs := []string{
		"", "_", ".", "a", "A", "a_", "a.", "a..", "a._b", "a_.b", "a__b", "a_b",
		"1", "1a", "a1", "a-b", "a b", "a.b", "a..b", ".a", "_a", "a_._b",
		"foo.bar.baz", "foo.bar.", "foo_bar", "foo__bar", "FOO.Bar_1",
		"café", "🍣", "a\x00b", "a\tb",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			normalized, ok := normalize(input)
			assert.Equal(t, ok && normalized == input, isNormalized(input))
		})
	}
}

func FuzzIsNormalizedMatchesNormalize(f *testing.F) {
	for input := range normalizedNames {
		f.Add(input)
	}
	for _, input := range unstorableNames {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, name string) {
		normalized, ok := normalize(name)

		assert.Equal(t, ok && normalized == name, isNormalized(name),
			"isNormalized disagrees with Normalize for %q", name)

		if !ok {
			assert.Equal(t, name, normalized)
			return
		}

		// Normalization must always produce a storable, stable name.
		assert.True(t, isNormalized(normalized),
			"Normalize produced a non-normalized name %q from %q", normalized, name)

		again, ok := normalize(normalized)
		assert.True(t, ok)
		assert.Equal(t, normalized, again, "Normalize is not idempotent for %q", name)
	})
}

func BenchmarkNormalizeAlreadyNormalized(b *testing.B) {
	name := "datadog.agent.some.metric_name.count"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		normalize(name)
	}
}

func BenchmarkNormalizeNeedsRewrite(b *testing.B) {
	name := "datadog.agent.some metric-name.count"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		normalize(name)
	}
}

// referenceNormalize is an independent transcription of NormMetricNameParse in
// dd-go model/metric.go, deliberately written as one straightforward pass with no
// fast path. It is the oracle for TestNormalizationMatchesReference, so that test
// pins this package against dd-go's behaviour rather than against itself.
//
// Keep it a transcription. If it is ever "simplified" to call the production code
// it stops being an oracle.
func referenceNormalize(name string) (string, bool) {
	if len(name) == 0 || len(name) > maxLength {
		return name, false
	}
	start := -1
	for i := 0; i < len(name); i++ {
		if isAlpha(name[i]) {
			start = i
			break
		}
	}
	if start < 0 {
		return name, false
	}

	res := make([]byte, 0, len(name))
	for i := start; i < len(name); i++ {
		switch c := name[i]; {
		case isAlphaNum(c):
			res = append(res, c)
		case c == '.':
			if res[len(res)-1] == '_' {
				res[len(res)-1] = '.'
			} else {
				res = append(res, '.')
			}
		default:
			if last := res[len(res)-1]; last != '.' && last != '_' {
				res = append(res, '_')
			}
		}
	}
	if res[len(res)-1] == '_' {
		res = res[:len(res)-1]
	}
	return string(res), true
}

// TestNormalizationMatchesReferenceExhaustive checks this package against the
// dd-go transcription above over every string up to length 6 drawn from an
// alphabet that reaches every branch: letter, digit, period, underscore, a byte
// that becomes an underscore, and a byte of a multi-byte rune. 137,257 cases,
// a few milliseconds.
//
// This exists because the fuzz targets in this file are weaker than they look:
// Bazel's test binary rejects -test.fuzz, so under `bazel test` they only ever
// replay their seed corpus. Exhaustive enumeration is the coverage that actually
// runs in CI.
//
// It also pins the equivalence that isNormalized's fast path depends on, since a
// name reported as already normalized must normalise to itself.
func TestNormalizationMatchesReferenceExhaustive(t *testing.T) {
	alphabet := []byte{'a', 'B', '1', '.', '_', '-', 0xC3}

	var buf []byte
	checked := 0
	var rec func(depth int)
	rec = func(depth int) {
		name := string(buf)
		checked++

		wantStr, wantOK := referenceNormalize(name)
		gotStr, gotOK := normalize(name)
		if wantOK != gotOK {
			t.Fatalf("storability disagrees for %q: reference %v, package %v",
				name, wantOK, gotOK)
		}
		if wantOK {
			if wantStr != gotStr {
				t.Fatalf("normalisation disagrees for %q: reference %q, package %q",
					name, wantStr, gotStr)
			}
			// The fast path in Matcher.Test skips the rewrite entirely for names
			// isNormalized accepts, so that must imply the rewrite is a no-op.
			if isNormalized(name) && gotStr != name {
				t.Fatalf("%q is reported normalized but normalises to %q", name, gotStr)
			}
			// And the output must itself be a fixed point.
			if !isNormalized(gotStr) {
				t.Fatalf("normalising %q produced %q, which is not normalized",
					name, gotStr)
			}
			// The bytes appendNormalizedFrom copies in bulk must be genuinely
			// shared with the result. This is the invariant that made `a_.b` and
			// `a___..b` fail while the output was still correct, because the
			// builder repaired the over-long prefix on the fly.
			if keep, identical, _ := normalizedPrefix(name); !identical {
				if keep > len(wantStr) || wantStr[:keep] != name[:keep] {
					t.Fatalf("keep=%d is not a shared prefix of %q and %q",
						keep, name, wantStr)
				}
			}
		}

		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			buf = append(buf, c)
			rec(depth - 1)
			buf = buf[:len(buf)-1]
		}
	}
	rec(6)
	t.Logf("checked %d strings against the dd-go transcription", checked)
}

// TestNormalizedPrefixIsCopyable pins the contract appendNormalizedFrom relies
// on: the first keep bytes really are what normalisation emits, so copying them
// without re-examining them is sound.
//
// The last two cases are the ones that caught bugs while this was being written.
// In both, the output was still correct because the builder repaired an over-long
// prefix, so only an explicit check on keep finds them.
func TestNormalizedPrefixIsCopyable(t *testing.T) {
	for _, name := range []string{
		"a-b", "a__b", "foo.bar-baz", "..foo", "7foo.bar",
		"normprobe.histo_.latency",
		"sportsdata.marketsettingsetl_.latency",
		"a_.b",    // the period overwrites the underscore keep would have included
		"a___..b", // the underscore at index 1 is overwritten four bytes later
	} {
		t.Run(name, func(t *testing.T) {
			keep, identical, ok := normalizedPrefix(name)
			require.True(t, ok, "expected %q to be storable", name)
			require.False(t, identical, "fixture must need a rewrite")

			full, _ := referenceNormalize(name)
			require.LessOrEqual(t, keep, len(full),
				"keep must not exceed the normalized length")
			assert.Equal(t, full[:keep], name[:keep],
				"the first %d bytes must survive verbatim", keep)
			assert.NotEqual(t, byte('_'), name[max(keep-1, 0)],
				"keep must not end in an underscore, which is never final")
		})
	}
}
