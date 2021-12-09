// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLTokenizerPosition(t *testing.T) {
	assert := assert.New(t)
	query := "SELECT username AS         person FROM users WHERE id=4"
	tok := NewSQLTokenizer(query, false, nil)
	tokenCount := 0
	for ; ; tokenCount++ {
		startPos := tok.Position()
		kind, buff := tok.Scan()
		if kind == EndChar {
			break
		}
		if kind == LexError {
			assert.Fail("experienced an unexpected lexer error")
		}
		assert.Equal(string(buff), query[startPos:tok.Position()])
		tok.SkipBlank()
	}
	assert.Equal(10, tokenCount)
}
