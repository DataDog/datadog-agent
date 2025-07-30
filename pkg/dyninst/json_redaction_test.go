// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

type matcher interface {
	matches(ptr jsontext.Pointer) bool
}

type replacer interface {
	replace(jsontext.Value) jsontext.Value
}

type replacerFunc func(jsontext.Value) jsontext.Value

func (f replacerFunc) replace(v jsontext.Value) jsontext.Value {
	return f(v)
}

type jsonRedactor struct {
	matcher  matcher
	replacer replacer
}

type exactMatcher string

func (m exactMatcher) matches(ptr jsontext.Pointer) bool {
	return string(ptr) == string(m)
}

type reMatcher regexp.Regexp

func (m *reMatcher) matches(ptr jsontext.Pointer) bool {
	return (*regexp.Regexp)(m).MatchString(string(ptr))
}

type replacement jsontext.Value

func (r replacement) replace(jsontext.Value) jsontext.Value {
	return jsontext.Value(r)
}

type prefixSuffixMatcher [2]string

func prefixMatcher(prefix string) prefixSuffixMatcher {
	return prefixSuffixMatcher{prefix, ""}
}

func (m prefixSuffixMatcher) matches(ptr jsontext.Pointer) bool {
	return strings.HasPrefix(string(ptr), m[0]) &&
		strings.HasSuffix(string(ptr), m[1])
}

func redactor(matcher matcher, replacer replacer) jsonRedactor {
	return jsonRedactor{
		matcher:  matcher,
		replacer: replacer,
	}
}

type stackFrame struct {
	Function   string `json:"function"`
	FileName   string `json:"fileName"`
	LineNumber any    `json:"lineNumber"`
}

var defaultRedactors = []jsonRedactor{
	redactor(
		(*reMatcher)(regexp.MustCompile(`^/debugger/snapshot/stack/[[:digit:]]+$`)),
		replacerFunc(func(v jsontext.Value) jsontext.Value {
			if v.Kind() != '{' {
				return v
			}
			d := json.NewDecoder(bytes.NewReader(v))
			d.UseNumber()
			var f stackFrame
			err := d.Decode(&f)
			if err != nil {
				return v
			}
			if !strings.HasPrefix(f.FileName, "runtime/asm_") {
				return v
			}
			f.FileName = "runtime/asm_[arch].s"
			f.LineNumber = "[lineNumber]"
			buf, err := json.Marshal(f)
			if err != nil {
				return v
			}
			return jsontext.Value(buf)
		}),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/id`),
		replacement(`"[id]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		exactMatcher(`/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/address"},
		replacement(`"[addr]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/entry/arguments/redactMyEntries", "/entries"},
		replacement(`"[redacted-entries]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/entries"},
		entriesSorter{},
	),
}

func redactJSON(t *testing.T, ptrPrefix jsontext.Pointer, input []byte, redactors []jsonRedactor) []byte {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(&buf, jsontext.WithIndent("  "), jsontext.WithIndentPrefix("  "))
	ptrPrefix = jsontext.Pointer(strings.TrimSuffix(string(ptrPrefix), "/"))
	stackPtr := func() jsontext.Pointer {
		return jsontext.Pointer(
			string(ptrPrefix) + "/" +
				strings.TrimPrefix(string(d.StackPointer()), "/"))
	}
	copyToken := func() (done bool) {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			return true
		}
		err = e.WriteToken(tok)
		require.NoError(t, err)
		return false
	}
	for {
		kind, idx := d.StackIndex(d.StackDepth())
		// If we're in an object and this is a key, copy it across.
		if kind == '{' && idx%2 == 0 || d.PeekKind() == ']' {
			require.False(t, copyToken(), "unexpected EOF")
		}
		ptr := stackPtr()
		var redacted []byte
		for _, redactor := range redactors {
			if redactor.matcher.matches(ptr) {
				v, err := d.ReadValue()
				require.NoError(t, err)
				redacted = redactor.replacer.replace(v)
				break
			}
		}

		// If we read a whole value, recursively redact it and write that out,
		// otherwise just copy the token across.
		if redacted != nil {
			switch jsontext.Value(redacted).Kind() {
			case '{', '[': // apply recursive redaction
				redacted = redactJSON(t, ptr, redacted, redactors)
			}
			require.NoError(t, e.WriteValue(redacted))
		} else if copyToken() {
			break
		}
	}
	return bytes.TrimSpace(buf.Bytes())
}

type entriesSorter struct{}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var notCapturedReason = []byte(`"notCapturedReason"`)

func compareNotCapturedReason(a, b []byte) int {
	return cmp.Compare(
		boolToInt(bytes.Contains(a, notCapturedReason)),
		boolToInt(bytes.Contains(b, notCapturedReason)),
	)
}

func (e entriesSorter) replace(v jsontext.Value) jsontext.Value {
	var entries [][2]jsontext.Value
	if err := json.Unmarshal(v, &entries); err != nil {
		return v // Return original value if unmarshal fails
	}
	slices.SortFunc(entries, func(a, b [2]jsontext.Value) int {
		return cmp.Or(
			compareNotCapturedReason(a[0], b[0]),
			bytes.Compare(a[0], b[0]),
		)
	})
	sorted, err := json.Marshal(entries)
	if err != nil {
		return v // Return original value if marshal fails
	}
	return sorted
}
