// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http contains the userspace portion of USM's HTTP monitoring code
package http

import (
	"bytes"
)

// URLQuantizer is responsible for quantizing URLs
type URLQuantizer struct {
	tokenizer *tokenizer
	buf       *bytes.Buffer
}

// NewURLQuantizer returns a new instance of a URLQuantizer
func NewURLQuantizer() *URLQuantizer {
	return &URLQuantizer{
		tokenizer: new(tokenizer),
		buf:       bytes.NewBuffer(nil),
	}
}

// Quantize path (eg /segment1/segment2/segment3) by doing the following:
// * If a segment contains only letters, we keep it as it is;
// * If a segment contains one or more digits or special characters, we replace it by '*'
// * If a segments represents an API version (eg. v123) we keep it as it is
//
// Note that the quantization happens *in-place* and the supplied argument byte
// slice is modified, so the returned value will still point to the same
// underlying byte array.
func (q *URLQuantizer) Quantize(path []byte) []byte {
	q.tokenizer.Reset(path)
	q.buf.Reset()
	replacements := 0

	for q.tokenizer.Next() {
		q.buf.WriteByte('/')
		tokenType, tokenValue := q.tokenizer.Value()
		if tokenType == tokenWildcard {
			replacements++
			q.buf.WriteByte('*')
			continue
		}

		q.buf.Write(tokenValue)
	}

	if replacements == 0 {
		return path
	}

	// Copy quantized path into original byte slice
	n := copy(path[:], q.buf.Bytes())

	return path[:n]
}

// tokenType represents a type of token handled by the `tokenizer`
type tokenType string

const (
	// tokenUnknown represents a token of type unknown
	tokenUnknown = "token:unknown"
	// tokenWildcard represents a token that contains digits or special chars
	tokenWildcard = "token:wildcard"
	// tokenString represents a token that contains only letters
	tokenString = "token:string"
	// tokenAPIVersion represents an API version (eg. v123)
	tokenAPIVersion = "token:api-version"
)

// tokenizer provides a stream of tokens for a given URL
type tokenizer struct {
	i, j int
	path []byte

	countAllowedChars int // a-Z, "-", "_"
	countNumbers      int // 0-9
	countSpecialChars int // anything else
}

// Reset underlying path being consumed
func (t *tokenizer) Reset(path []byte) {
	t.i = 0
	t.j = 0
	t.path = path
}

// Next attempts to parse the next token, and returns true if a token was read
func (t *tokenizer) Next() bool {
	t.countNumbers = 0
	t.countAllowedChars = 0
	t.countSpecialChars = 0
	t.i = t.j + 1

	if t.i >= len(t.path)-1 {
		return false
	}

	for t.j = t.i; t.j < len(t.path); t.j++ {
		c := t.path[t.j]

		if c == '/' {
			break
		} else if c >= '0' && c <= '9' {
			t.countNumbers++
		} else if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '_' {
			t.countAllowedChars++
		} else {
			t.countSpecialChars++
		}
	}

	return true
}

// Value returns the current token along with it's byte value
// Note that the byte value is only valid until the next call to `Reset()`
func (t *tokenizer) Value() (tokenType, []byte) {
	if t.i < 0 || t.j > len(t.path) || t.i >= t.j {
		return tokenUnknown, nil
	}

	return t.getType(), t.path[t.i:t.j]
}

func (t *tokenizer) getType() tokenType {
	if t.countAllowedChars == 1 && t.countNumbers > 0 && t.path[t.i] == 'v' {
		return tokenAPIVersion
	}

	if t.countSpecialChars > 0 || t.countNumbers > 0 {
		return tokenWildcard
	}

	if t.countAllowedChars > 0 && t.countSpecialChars == 0 && t.countNumbers == 0 {
		return tokenString
	}

	return tokenUnknown
}
