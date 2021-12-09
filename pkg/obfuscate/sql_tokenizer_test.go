// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTokenizerIndex(t *testing.T) {
	assert := assert.New(t)

	query := "SELECT username AS         person FROM users WHERE id=4"
	tokenizer := NewSQLTokenizer(query, false, nil)
	for {
		startPos := tokenizer.Position()
		kind, buff := tokenizer.Scan()
		if kind == EndChar {
			break
		}
		if kind == LexError {
			assert.Fail("experienced an unexpected lexer error")
		}

		assert.Equal(string(buff), query[startPos:tokenizer.Position()])
		tokenizer.SkipBlank()
	}

}
