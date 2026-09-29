// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import "slices"

// OwnedTokens is an immutable token result: the tokens and their start byte
// offsets. It owns its memory, so it may be kept after the Tokenizer that
// produced it is reused, and read from several goroutines at once. Nothing in
// its API writes to it; Tokens and Indices return copies.
//
// Build one with BorrowedTokens.Own. The zero value is a valid empty result.
type OwnedTokens struct {
	tokens  []Token
	indices []int
}

// Own copies the view into an OwnedTokens. Indices are copied as they are: a
// view from TokenizeBorrowed has one per token, while a view built without
// indices (for example by Retained) gives a result without indices.
func (b BorrowedTokens) Own() OwnedTokens {
	if len(b.tokens) == 0 {
		return OwnedTokens{}
	}
	return OwnedTokens{tokens: slices.Clone(b.tokens), indices: slices.Clone(b.indices)}
}

// Len reports the number of tokens.
func (o OwnedTokens) Len() int { return len(o.tokens) }

// Empty reports whether there are no tokens.
func (o OwnedTokens) Empty() bool { return len(o.tokens) == 0 }

// HasIndices reports whether the result carries a start offset for every
// token.
func (o OwnedTokens) HasIndices() bool { return len(o.indices) == len(o.tokens) }

// At returns token i. It panics if i is out of range.
func (o OwnedTokens) At(i int) Token { return o.tokens[i] }

// Start returns the start byte offset of token i. It panics if i is out of
// range or the result has no indices.
func (o OwnedTokens) Start(i int) int { return o.indices[i] }

// Tokens returns a copy of the tokens.
func (o OwnedTokens) Tokens() []Token { return slices.Clone(o.tokens) }

// Indices returns a copy of the start byte offsets (nil when the result has
// none).
func (o OwnedTokens) Indices() []int { return slices.Clone(o.indices) }

// Limit returns the tokens that start before maxBytes (maxBytes <= 0 means no
// limit), with the same rule as BorrowedTokens.Limit. The result shares memory
// with o, which is safe because neither can be modified.
func (o OwnedTokens) Limit(maxBytes int) OwnedTokens {
	if maxBytes <= 0 {
		return o
	}
	for i, idx := range o.indices {
		if idx >= maxBytes {
			return OwnedTokens{tokens: o.tokens[:i:i], indices: o.indices[:i:i]}
		}
	}
	return o
}
