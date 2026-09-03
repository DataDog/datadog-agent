// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalizeTagName is the string-returning form of NormalizeTagNameAppend, for
// tests that assert on the normalized name. Production code normalizes into a
// stack buffer instead.
func normalizeTagName(name string) (string, bool) {
	got, ok := NormalizeTagNameAppend(make([]byte, 0, MaxNormalizedTagLength), name)
	if !ok {
		return "", false
	}
	return string(got), true
}

// normalizedTagNames covers the tag name rules. Expectations are the *name* the
// intake stores, so where an input also happens to be a whole valueless tag they
// differ from dd-go's tables by the trailing underscore that dd-go strips from the
// end of a tag; ddgoValuelessTagNames below covers that composition.
var normalizedTagNames = map[string]string{
	// Already normalized, must be returned untouched.
	"env":            "env",
	"kube_namespace": "kube_namespace",
	"host-name":      "host-name",
	"my.tag":         "my.tag",
	"path/to":        "path/to",
	"tag1":           "tag1",

	// Lowercased, unlike a metric name.
	"Env":     "env",
	"MyTag":   "mytag",
	"KUBE_NS": "kube_ns",

	// Hyphens and slashes survive, unlike in a metric name.
	"my-tag":    "my-tag",
	"My-Tag":    "my-tag",
	"a/b-c.d":   "a/b-c.d",
	"kube-app":  "kube-app",
	"foo_-_bar": "foo_-_bar",

	// Anything else becomes a single underscore, including literal underscores.
	"foo bar":         "foo_bar",
	"foo  bar":        "foo_bar",
	"foo__bar":        "foo_bar",
	"foo_%bar":        "foo_bar",
	"foo_=bar":        "foo_bar",
	"foo_@(bar":       "foo_bar",
	"foo_(bar_+_baz)": "foo_bar_baz_",

	// A trailing underscore is kept, because the intake only strips one from the
	// very end of the whole tag: the name in `foo_:value` is stored as `foo_`.
	"foo!":     "foo_",
	"foo_*":    "foo_",
	"foo_":     "foo_",
	"foo__":    "foo_",
	"foo bar ": "foo_bar_",
	"tag🍣":     "tag_",

	// An underscore next to a period is kept, unlike in a metric name.
	"a._b": "a._b",
	"a_.b": "a_.b",

	// Everything before the first letter is dropped, including digits.
	"1tag":     "tag",
	"...tag":   "tag",
	"_tag":     "tag",
	"-tag":     "tag",
	"5-2 tags": "tags",

	// Non-ASCII letters are lowercased and kept.
	"café":   "café",
	"CAFÉ":   "café",
	"Straße": "straße",
	// Non-letters are still underscores.
	"tag🍣end": "tag_end",
}

// ddgoValuelessTagNames mirrors entries of the `normalizeTagTests` table in dd-go
// (`model/tags_test.go`) whose input is a whole tag with no value, so a divergence
// between the two implementations shows up as a failure here. These expectations
// include the trailing underscore strip dd-go applies at the end of a tag, which
// is why they go through normalizeValuelessTagName.
var ddgoValuelessTagNames = map[string]string{
	"foo_%bar":        "foo_bar",
	"foo_=bar":        "foo_bar",
	"foo_'bar":        "foo_bar",
	"foo_\"bar":       "foo_bar",
	"foo_\\bar":       "foo_bar",
	"foo_@(bar":       "foo_bar",
	"foo_*":           "foo",
	"foo_(bar_+_baz)": "foo_bar_baz",
	"FOO":             "foo",
	"foo bar":         "foo_bar",
}

// unstorableTagNames are names that normalize to nothing, so the intake drops the
// tag entirely.
var unstorableTagNames = []string{
	"",
	"_",
	"...",
	"123",
	"127.0.0.0",
	"🍣",
	"-",
	"1.2-3/4",
}

func TestNormalizeTagName(t *testing.T) {
	for input, expected := range normalizedTagNames {
		t.Run(input, func(t *testing.T) {
			actual, ok := normalizeTagName(input)
			require.True(t, ok)
			assert.Equal(t, expected, actual)
		})
	}
}

// normalizeValuelessTagName composes the name rules with the rule a caller holding
// a tag that carries no value has to apply, since the name then ends the tag: see
// NormalizeTagNameAppend, and hashTagName in comp/filterlist/impl for the caller
// that does it. It exists so this package can be checked against dd-go's whole-tag
// expectations.
func normalizeValuelessTagName(tag string) (string, bool) {
	name, ok := normalizeTagName(tag)
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(name, "_"), true
}

// TestNormalizeValuelessTagNameMatchesDDGo checks the composed rules against dd-go's
// own expectations for whole tags without a value.
func TestNormalizeValuelessTagNameMatchesDDGo(t *testing.T) {
	for input, expected := range ddgoValuelessTagNames {
		t.Run(input, func(t *testing.T) {
			actual, ok := normalizeValuelessTagName(input)
			require.True(t, ok)
			assert.Equal(t, expected, actual)
		})
	}
}

// TestNormalizeTagNameKeepsTrailingUnderscore pins the rule that the name of a tag
// carrying a value keeps its trailing underscore, because the intake only strips
// one from the very end of a tag: `my_tag_:value` is stored with the name
// `my_tag_`, while the valueless tag `my_tag_` is stored as `my_tag`.
func TestNormalizeTagNameKeepsTrailingUnderscore(t *testing.T) {
	for _, name := range []string{"my_tag_", "my tag ", "my_tag_*"} {
		t.Run(name, func(t *testing.T) {
			actual, ok := normalizeTagName(name)
			require.True(t, ok)
			assert.Equal(t, "my_tag_", actual)
			assert.True(t, IsNormalizedASCIITagName(actual),
				"a name ending in an underscore is normalized")

			valueless, ok := normalizeValuelessTagName(name)
			require.True(t, ok)
			assert.Equal(t, "my_tag", valueless)
		})
	}
}

func TestNormalizeTagNameUnstorable(t *testing.T) {
	for _, input := range unstorableTagNames {
		t.Run(input, func(t *testing.T) {
			_, ok := normalizeTagName(input)
			assert.False(t, ok)
		})
	}
}

func TestNormalizeTagNameIsIdempotent(t *testing.T) {
	for input := range normalizedTagNames {
		t.Run(input, func(t *testing.T) {
			once, ok := normalizeTagName(input)
			require.True(t, ok)
			twice, ok := normalizeTagName(once)
			require.True(t, ok)
			assert.Equal(t, once, twice)
		})
	}
}

// TestNormalizeTagNameColon documents that a colon is not special here: callers
// are expected to have split the tag already, and a name still holding one is
// rewritten rather than truncated at it.
func TestNormalizeTagNameColon(t *testing.T) {
	actual, ok := normalizeTagName("env:prod")
	require.True(t, ok)
	assert.Equal(t, "env_prod", actual)
}

// TestNormalizeTagNameTruncates checks the length rule, which differs from metric
// names: an over-long tag name is cut down rather than rejected.
func TestNormalizeTagNameTruncates(t *testing.T) {
	atLimit := strings.Repeat("a", MaxTagLength)

	actual, ok := normalizeTagName(atLimit)
	require.True(t, ok)
	assert.Equal(t, atLimit, actual)

	actual, ok = normalizeTagName(atLimit + "bbb")
	require.True(t, ok)
	assert.Equal(t, atLimit, actual, "an over-long name is truncated, not rejected")

	// Truncation can leave a trailing underscore, which is kept like any other.
	actual, ok = normalizeTagName(strings.Repeat("a", MaxTagLength-1) + "_bbb")
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("a", MaxTagLength-1)+"_", actual)

	// A multi-byte code point straddling the limit overshoots it, as it does in
	// the intake, but never past MaxNormalizedTagLength.
	actual, ok = normalizeTagName(strings.Repeat("a", MaxTagLength-1) + "é" + "b")
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("a", MaxTagLength-1)+"é", actual)
	assert.LessOrEqual(t, len(actual), MaxNormalizedTagLength)
}

// TestNormalizeTagNameAppendReusesBuffer pins the two properties callers depend on
// for the stack buffer: existing content is left alone, and a buffer with
// MaxNormalizedTagLength spare capacity is never grown.
func TestNormalizeTagNameAppendReusesBuffer(t *testing.T) {
	buf := make([]byte, 0, MaxNormalizedTagLength)
	buf = append(buf, "keep:"...)

	got, ok := NormalizeTagNameAppend(buf, "My Tag")
	require.True(t, ok)
	assert.Equal(t, "keep:my_tag", string(got))

	// An unstorable name leaves the buffer untouched.
	got, ok = NormalizeTagNameAppend(buf, "123")
	require.False(t, ok)
	assert.Equal(t, "keep:", string(got))

	var stack [MaxNormalizedTagLength]byte
	long := strings.Repeat("a-", MaxTagLength)
	allocs := testing.AllocsPerRun(100, func() {
		NormalizeTagNameAppend(stack[:0], long)
	})
	assert.Zero(t, allocs, "normalizing into a MaxNormalizedTagLength buffer must not reallocate")
}

func TestIsNormalizedASCIITagName(t *testing.T) {
	for input, expected := range normalizedTagNames {
		t.Run(input, func(t *testing.T) {
			if !isASCII(input) {
				// The predicate is one-sided: a non-ASCII name is always
				// reported as not normalized, even when it is a fixed point.
				assert.False(t, IsNormalizedASCIITagName(input))
				return
			}
			assert.Equal(t, input == expected, IsNormalizedASCIITagName(input))
		})
	}

	for _, input := range unstorableTagNames {
		t.Run(input, func(t *testing.T) {
			assert.False(t, IsNormalizedASCIITagName(input))
		})
	}

	// A colon means the caller has not split the tag, so it is never a
	// normalized tag name.
	assert.False(t, IsNormalizedASCIITagName("env:prod"))
	assert.False(t, IsNormalizedASCIITagName("env"+strings.Repeat("a", MaxTagLength)))
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// referenceNormalizeTagName is an independent transcription of `NormalizeTag` in
// dd-go `model/tags.go`, restricted to a name with no colon (see
// NormalizeTagNameAppend) and deliberately written without a fast path. It is the
// oracle for TestTagNameNormalizationMatchesReferenceExhaustive, so that test pins
// this package against dd-go's behaviour rather than against itself.
//
// dd-go's trailing underscore strip is deliberately not transcribed: it applies to
// the end of the whole tag, which a name followed by a value is not. Callers
// holding a valueless tag apply it themselves, and that is covered where they do.
func referenceNormalizeTagName(name string) (string, bool) {
	var buf []byte
	lastWasUnderscore := false

	for i, c := range name {
		if len(buf) >= MaxTagLength {
			break
		}
		if i > 2*MaxTagLength {
			break
		}

		switch {
		case unicode.IsLetter(c):
			buf = utf8.AppendRune(buf, unicode.ToLower(c))
			lastWasUnderscore = false
		case len(buf) == 0:
			// Dropped: nothing can start the name but a letter.
		case unicode.IsDigit(c) || c == '.' || c == '/' || c == '-':
			buf = utf8.AppendRune(buf, c)
			lastWasUnderscore = false
		case !lastWasUnderscore:
			buf = append(buf, '_')
			lastWasUnderscore = true
		}
	}

	if len(buf) == 0 {
		return "", false
	}
	return string(buf), true
}

// TestTagNameNormalizationMatchesReferenceExhaustive checks this package against
// the dd-go transcription above over every string up to length 5 drawn from an
// alphabet that reaches every branch: lower and upper ASCII letters, a digit, the
// three punctuation characters that survive, an underscore, a byte that becomes an
// underscore, and a byte of a multi-byte rune. 108,801 cases, a few milliseconds.
//
// This is the counterpart of TestNormalizationMatchesReferenceExhaustive for
// metric names, and exists for the same reason: `bazel test` cannot run fuzz
// targets, so enumeration is the coverage that actually runs in CI.
//
// It also pins the one-sided guarantee IsNormalizedASCIITagName makes, since a
// name it accepts must normalize to itself.
func TestTagNameNormalizationMatchesReferenceExhaustive(t *testing.T) {
	alphabet := []byte{'a', 'B', '1', '.', '/', '-', '_', ' ', 0xC3}

	var buf []byte
	checked := 0
	var rec func(depth int)
	rec = func(depth int) {
		name := string(buf)
		checked++

		wantStr, wantOK := referenceNormalizeTagName(name)
		gotStr, gotOK := normalizeTagName(name)
		if wantOK != gotOK {
			t.Fatalf("storability disagrees for %q: reference %v, package %v",
				name, wantOK, gotOK)
		}
		if wantOK {
			if wantStr != gotStr {
				t.Fatalf("normalisation disagrees for %q: reference %q, package %q",
					name, wantStr, gotStr)
			}
			// Callers skip the rewrite for names the predicate accepts, so that
			// must imply the rewrite is a no-op.
			if IsNormalizedASCIITagName(name) && gotStr != name {
				t.Fatalf("%q is reported normalized but normalises to %q", name, gotStr)
			}
			// And an all-ASCII output must itself be a fixed point, otherwise
			// the two sides of a comparison could disagree.
			if isASCII(gotStr) && !IsNormalizedASCIITagName(gotStr) {
				t.Fatalf("normalising %q produced %q, which is not normalized",
					name, gotStr)
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
	rec(5)
	t.Logf("checked %d strings against the dd-go transcription", checked)
}

func FuzzNormalizeTagName(f *testing.F) {
	for input := range normalizedTagNames {
		f.Add(input)
	}
	for _, input := range unstorableTagNames {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, name string) {
		wantStr, wantOK := referenceNormalizeTagName(name)
		gotStr, gotOK := normalizeTagName(name)

		assert.Equal(t, wantOK, gotOK, "storability disagrees for %q", name)
		if !gotOK {
			return
		}
		assert.Equal(t, wantStr, gotStr, "normalisation disagrees for %q", name)

		if IsNormalizedASCIITagName(name) {
			assert.Equal(t, name, gotStr, "%q is reported normalized but was rewritten", name)
		}

		again, ok := normalizeTagName(gotStr)
		assert.True(t, ok)
		assert.Equal(t, gotStr, again, "normalizing is not idempotent for %q", name)
		assert.LessOrEqual(t, len(gotStr), MaxNormalizedTagLength)
	})
}

func BenchmarkNormalizeTagNameAlreadyNormalized(b *testing.B) {
	name := "kube_namespace"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		normalizeTagName(name)
	}
}

func BenchmarkNormalizeTagNameNeedsRewrite(b *testing.B) {
	name := "Kube Namespace"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		normalizeTagName(name)
	}
}
