// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

func TestTokenizerWithoutLiterals(t *testing.T) {
	tokenizer := NewTokenizer(1000)

	// Test that tokens are created without literal values
	toks, _ := tokenizer.Tokenize([]byte("app_key"))

	assert.Equal(t, 3, len(toks))
	assert.Equal(t, tokens.C3, toks[0].Kind)
	assert.Equal(t, "", toks[0].Lit) // No literal
	assert.Equal(t, tokens.Underscore, toks[1].Kind)
	assert.Equal(t, "", toks[1].Lit) // No literal
	assert.Equal(t, tokens.C3, toks[2].Kind)
	assert.Equal(t, "", toks[2].Lit) // No literal
}

func TestTokenizerWithLiterals(t *testing.T) {
	tokenizer := NewTokenizerWithLiterals(1000)

	// Test that tokens are created with literal values
	toks, _ := tokenizer.Tokenize([]byte("app_key"))

	assert.Equal(t, 3, len(toks))
	assert.Equal(t, tokens.C3, toks[0].Kind)
	assert.Equal(t, "app", toks[0].Lit) // Has literal
	assert.Equal(t, tokens.Underscore, toks[1].Kind)
	assert.Equal(t, "_", toks[1].Lit) // Has literal
	assert.Equal(t, tokens.C3, toks[2].Kind)
	assert.Equal(t, "key", toks[2].Lit) // Has literal
}

func TestSpecialTokensWithoutLiterals(t *testing.T) {
	tokenizer := NewTokenizer(1000)

	// Test special tokens work without literals
	toks, _ := tokenizer.Tokenize([]byte("Jan 2PM"))

	// "Jan" -> Month, " " -> Space, "2" -> D1, "PM" -> Apm
	assert.Equal(t, 4, len(toks))
	assert.Equal(t, tokens.Month, toks[0].Kind)
	assert.Equal(t, "", toks[0].Lit) // No literal
	assert.Equal(t, tokens.Space, toks[1].Kind)
	assert.Equal(t, tokens.D1, toks[2].Kind)
	assert.Equal(t, "", toks[2].Lit) // No literal (digit)
	assert.Equal(t, tokens.Apm, toks[3].Kind)
	assert.Equal(t, "", toks[3].Lit) // No literal for PM
}

func TestSpecialTokensWithLiterals(t *testing.T) {
	tokenizer := NewTokenizerWithLiterals(1000)

	// Test special tokens work with literals
	toks, _ := tokenizer.Tokenize([]byte("Jan"))

	assert.Equal(t, 1, len(toks))
	assert.Equal(t, tokens.Month, toks[0].Kind)
	assert.Equal(t, "Jan", toks[0].Lit) // Has literal "Jan"
}
