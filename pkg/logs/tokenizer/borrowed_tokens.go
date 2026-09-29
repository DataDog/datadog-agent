// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import "slices"

// BorrowedTokens is a view over a Tokenizer's scratch buffers, valid only until
// the next tokenization on that Tokenizer. The type enforces the borrow/copy
// contract: a borrowed view cannot be assigned to an owned []Token field, so
// retaining tokens must go through Clone (or Retained). The zero value is a
// valid empty view.
type BorrowedTokens struct {
	tokens  []Token
	indices []int
}

// NewBorrowedTokens wraps token/index slices in a view without copying. The
// caller keeps ownership of both slices and must not modify them while the view
// is in use.
func NewBorrowedTokens(tokens []Token, indices []int) BorrowedTokens {
	return BorrowedTokens{tokens: tokens, indices: indices}
}

// Borrow returns the tokens for synchronous reads only; do not retain them.
func (b BorrowedTokens) Borrow() []Token { return b.tokens }

// Indices returns the per-token start byte offsets, borrowed like Borrow.
func (b BorrowedTokens) Indices() []int { return b.indices }

// Clone returns an owned copy of the tokens, safe to retain.
func (b BorrowedTokens) Clone() []Token { return slices.Clone(b.tokens) }

// Len reports the number of tokens.
func (b BorrowedTokens) Len() int { return len(b.tokens) }

// Empty reports whether there are no tokens.
func (b BorrowedTokens) Empty() bool { return len(b.tokens) == 0 }

// Retained returns a view backed by an owned copy, safe to store across calls.
// Indices are dropped; only the labeler window uses them, before any retention.
func (b BorrowedTokens) Retained() BorrowedTokens {
	return BorrowedTokens{tokens: slices.Clone(b.tokens)}
}

// Limit returns the prefix of the view whose tokens start before maxBytes
// (maxBytes <= 0 means no limit), giving the labeler a narrower window than the
// sampler. The result is a sub-view over the same backing.
//
// Limit is not the same as tokenizing only the first maxBytes. A token that
// starts before maxBytes keeps the shape it has in the full input: a run that
// crosses the boundary keeps its full length bucket (D5, not D2), and an IPv4
// address that crosses it stays one IPv4 token.
func (b BorrowedTokens) Limit(maxBytes int) BorrowedTokens {
	if maxBytes <= 0 {
		return b
	}
	for i, idx := range b.indices {
		if idx >= maxBytes {
			return BorrowedTokens{tokens: b.tokens[:i], indices: b.indices[:i]}
		}
	}
	return b
}
