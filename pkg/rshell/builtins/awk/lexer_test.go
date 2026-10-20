// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeAwkHexEscapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`\x4`, "\x04"},
		{`\x41`, "A"},
		{`\x4142`, "A42"},
		{`\x00Z`, "\x00Z"},
		{`\xff`, "\xff"},
		{`\xZ`, "xZ"},
	}
	for _, tc := range tests {
		assert.Equal(t, []byte(tc.want), []byte(DecodeAwkEscapes(tc.input)), tc.input)
	}
}

func TestLexRegexKeepsPOSIXBracketSubexpressionsNested(t *testing.T) {
	tokens, err := lex(`BEGIN { print /[[:alpha:]/]/, /[[.x.]/]/, /[[=x=]/]/ }`)
	if !assert.NoError(t, err) {
		return
	}

	var regexes []string
	for _, tok := range tokens {
		if tok.kind == tokRegex {
			regexes = append(regexes, tok.lit)
		}
	}
	assert.Equal(t, []string{"[[:alpha:]/]", "[[.x.]/]", "[[=x=]/]"}, regexes)
}
