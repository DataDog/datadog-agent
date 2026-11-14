// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagname

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readVectors parses a vector file (SPEC.md §9) into its fields. Quoted fields
// are unquoted; bare words are returned with a leading '!' so they can never be
// confused with a quoted value.
func readVectors(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	require.NoError(t, err)
	defer f.Close()

	var vectors [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 1; sc.Scan(); line++ {
		var fields []string
		s := strings.TrimSpace(sc.Text())
		for s != "" && s[0] != '#' {
			if s[0] == '"' {
				q, err := strconv.QuotedPrefix(s)
				require.NoError(t, err, "%s:%d", name, line)
				v, err := strconv.Unquote(q)
				require.NoError(t, err, "%s:%d", name, line)
				fields = append(fields, v)
				s = strings.TrimSpace(s[len(q):])
				continue
			}
			word, rest, _ := strings.Cut(s, " ")
			fields = append(fields, "!"+word)
			s = strings.TrimSpace(rest)
		}
		if len(fields) > 0 {
			vectors = append(vectors, fields)
		}
	}
	require.NoError(t, sc.Err())
	require.NotEmpty(t, vectors, name)
	return vectors
}

func TestVectors(t *testing.T) {
	for _, v := range readVectors(t, "vectors.txt") {
		require.Len(t, v, 3, "vector %+q", v)
		in, normalized, key := v[0], v[1], v[2]

		assert.Equal(t, normalized, Normalize(in), "Normalize(%+q)", in)
		assert.Equal(t, "pre"+normalized, string(NormalizeAppend([]byte("pre"), in)), "NormalizeAppend(%+q)", in)
		assert.Equal(t, key, Key(in), "Key(%+q)", in)
		assert.Equal(t, "pre"+key, string(KeyAppend([]byte("pre"), in)), "KeyAppend(%+q)", in)
		checkProperties(t, in)
	}
}

func TestRuleVectors(t *testing.T) {
	for _, v := range readVectors(t, "rule_vectors.txt") {
		require.Len(t, v, 2, "vector %+q", v)
		in, want := v[0], v[1]

		key, ok := RuleKey(in)
		if want == "!reject" {
			assert.False(t, ok, "RuleKey(%+q) = %+q, want reject", in, key)
			continue
		}
		assert.True(t, ok, "RuleKey(%+q) rejected, want %+q", in, want)
		assert.Equal(t, want, key, "RuleKey(%+q)", in)
	}
}

// checkProperties asserts the properties of SPEC.md §8 for one input.
func checkProperties(t testing.TB, in string) {
	t.Helper()
	normalized := Normalize(in)
	key := Key(in)

	// P1: normalization is idempotent.
	if again := Normalize(normalized); again != normalized {
		t.Fatalf("P1: Normalize(%+q) = %+q, but Normalize of that is %+q", in, normalized, again)
	}

	// P2: the key is the part of the normalized tag before its first ':'.
	if want, _, _ := strings.Cut(normalized, ":"); key != want {
		t.Fatalf("P2: Key(%+q) = %+q, want prefix %+q of %+q", in, key, want, normalized)
	}

	// P3: the key of a normalized tag is the key of the tag.
	if k := Key(normalized); k != key {
		t.Fatalf("P3: Key(%+q) = %+q, but Key(Normalize) = %+q", in, key, k)
	}

	// P4: every stored key, including the empty key, is a fixed point of RuleKey.
	if k, ok := RuleKey(key); !ok || k != key {
		t.Fatalf("P4: RuleKey(%+q) = %+q, %v for the stored key of %+q", key, k, ok, in)
	}

	// P5: the fast paths agree with the state machine whenever they apply.
	if isNormalizedASCII(in) {
		if s := string(scan(nil, in, false)); s != in {
			t.Fatalf("P5: isNormalizedASCII(%+q) but the state machine gives %+q", in, s)
		}
	}
	if k, ok := fastKey(in); ok {
		if s := string(scan(nil, in, true)); s != k {
			t.Fatalf("P5: fastKey(%+q) = %+q but the state machine gives %+q", in, k, s)
		}
	}

	// P6: output shape.
	if len(normalized) > MaxOutputLength || len(key) > MaxOutputLength {
		t.Fatalf("P6: output of %+q too long: %d, %d", in, len(normalized), len(key))
	}
	if strings.HasSuffix(normalized, "_") {
		t.Fatalf("P6: Normalize(%+q) = %+q ends in '_'", in, normalized)
	}
	if strings.ContainsRune(key, ':') {
		t.Fatalf("P6: Key(%+q) = %+q contains ':'", in, key)
	}
	if !utf8.ValidString(normalized) {
		t.Fatalf("P6: Normalize(%+q) = %+q is not valid UTF-8", in, normalized)
	}
	if normalized != "" {
		r, _ := utf8.DecodeRuneInString(normalized)
		if r != ':' && !unicode.IsLetter(r) {
			t.Fatalf("P6: Normalize(%+q) = %+q starts with %q", in, normalized, r)
		}
	}
}

func TestPropertiesOnEdgeCases(t *testing.T) {
	inputs := []string{
		"", ":", "_", "a_", "_a", "a_:b", ":_", "1:2", "\xff", "a\xff", strings.Repeat("a", 199) + "_:b",
		strings.Repeat("_a", 150), strings.Repeat("é", 150), strings.Repeat("A:", 150),
	}
	for _, in := range inputs {
		checkProperties(t, in)
	}
}

func FuzzProperties(f *testing.F) {
	f.Add("env:prod")
	f.Add("Env:Prod")
	f.Add("a_:b")
	f.Add("123:foo")
	f.Add("é:x")
	f.Add("a\xffb")
	f.Add(strings.Repeat("a", 199) + "\U00010400")
	f.Fuzz(func(t *testing.T, in string) {
		checkProperties(t, in)
	})
}

func TestRuleKeyRejectsColons(t *testing.T) {
	for _, in := range []string{":", "a:", ":a", "a:b", "Env:Prod"} {
		_, ok := RuleKey(in)
		assert.False(t, ok, "RuleKey(%+q)", in)
	}
}

func TestKeyDoesNotAllocate(t *testing.T) {
	normalized := "kube_namespace:default"
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = Key(normalized) }))

	var buf [MaxOutputLength]byte
	for _, in := range []string{normalized, "Kube Namespace:Default", strings.Repeat("\U00010400", 100)} {
		assert.Zero(t, testing.AllocsPerRun(100, func() { _ = KeyAppend(buf[:0], in) }), "KeyAppend(%+q)", in)
	}
}

func BenchmarkKey(b *testing.B) {
	for _, bc := range []struct{ name, tag string }{
		{"normalized", "kube_namespace:default"},
		{"normalized-long-value", "image_id:sha256:" + strings.Repeat("0123456789abcdef", 4)},
		{"uppercase", "Kube_Namespace:default"},
		{"unicode", "équipe:développement"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			var buf [MaxOutputLength]byte
			for b.Loop() {
				_ = KeyAppend(buf[:0], bc.tag)
			}
		})
	}
}
