// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"errors"
	"io"
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

type replacement jsontext.Value

func (r replacement) replace(jsontext.Value) jsontext.Value {
	return jsontext.Value(r)
}

type prefixSuffixMatcher [2]string

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

var defaultRedactors = []jsonRedactor{
	redactor(
		exactMatcher(`/debugger/snapshot/stack`),
		replacement(`"[stack-unredact-me]"`),
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
}

func redactJSON(t *testing.T, input []byte, redactors []jsonRedactor) (redacted []byte) {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(&buf, jsontext.WithIndent("  "), jsontext.WithIndentPrefix("  "))
	for {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		kind, idx := d.StackIndex(d.StackDepth())
		err = e.WriteToken(tok)
		require.NoError(t, err)
		if kind != '{' || idx%2 == 0 {
			continue
		}
		ptr := d.StackPointer()
		for _, redactor := range redactors {
			if redactor.matcher.matches(ptr) {
				v, err := d.ReadValue()
				require.NoError(t, err)
				err = e.WriteValue(redactor.replacer.replace(v))
				require.NoError(t, err)
				break
			}
		}
	}
	return bytes.TrimSpace(buf.Bytes())
}
