// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnedTokensMatchesTokenize(t *testing.T) {
	input := []byte("2024-01-15 ERROR 10.0.0.1 request failed")

	wantTokens, wantIndices := NewTokenizer(0).Tokenize(input)
	owned := NewTokenizer(0).TokenizeBorrowed(input).Own()

	assert.Equal(t, wantTokens, owned.Tokens())
	assert.Equal(t, wantIndices, owned.Indices())
	require.Equal(t, len(wantTokens), owned.Len())
	assert.False(t, owned.Empty())
	assert.True(t, owned.HasIndices())
	for i := range wantTokens {
		assert.Equal(t, wantTokens[i], owned.At(i))
		assert.Equal(t, wantIndices[i], owned.Start(i))
	}
}

func TestOwnedTokensSurvivesTokenizerReuse(t *testing.T) {
	tok := NewTokenizer(0)
	borrowed := tok.TokenizeBorrowed([]byte("abc 123"))
	want := borrowed.Clone()
	wantIndices := append([]int(nil), borrowed.Indices()...)
	owned := borrowed.Own()

	// Reusing the tokenizer overwrites the borrowed scratch buffers.
	tok.TokenizeBorrowed([]byte("!!!! zzzzzzzz 9"))

	assert.Equal(t, want, owned.Tokens())
	assert.Equal(t, wantIndices, owned.Indices())
}

func TestOwnedTokensAccessorsReturnCopies(t *testing.T) {
	owned := NewTokenizer(0).TokenizeBorrowed([]byte("abc 123")).Own()
	want := owned.Tokens()
	wantIndices := owned.Indices()

	gotTokens := owned.Tokens()
	gotTokens[0] = End
	gotIndices := owned.Indices()
	gotIndices[0] = 99

	assert.Equal(t, want, owned.Tokens())
	assert.Equal(t, wantIndices, owned.Indices())
}

func TestOwnedTokensEmpty(t *testing.T) {
	for name, owned := range map[string]OwnedTokens{
		"zero value":  {},
		"empty input": NewTokenizer(0).TokenizeBorrowed(nil).Own(),
		"empty view":  NewBorrowedTokens([]Token{}, []int{}).Own(),
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, owned.Empty())
			assert.Equal(t, 0, owned.Len())
			assert.True(t, owned.HasIndices())
			assert.Empty(t, owned.Tokens())
			assert.Empty(t, owned.Indices())
			assert.True(t, owned.Limit(5).Empty())
		})
	}
}

func TestOwnedTokensWithoutIndices(t *testing.T) {
	owned := NewBorrowedTokens([]Token{C3, Space, D3}, nil).Own()

	assert.Equal(t, 3, owned.Len())
	assert.False(t, owned.HasIndices())
	assert.Nil(t, owned.Indices())
	// Same as BorrowedTokens.Limit: without indices nothing is cut.
	assert.Equal(t, 3, owned.Limit(1).Len())
	assert.Panics(t, func() { owned.Start(0) })
}

func TestOwnedTokensLimitMatchesBorrowedLimit(t *testing.T) {
	input := []byte("abc 12345 10.0.0.1 up WARN")
	for _, maxBytes := range []int{-1, 0, 1, 3, 4, 6, 9, 12, 20, len(input), 1000} {
		borrowed := NewTokenizer(0).TokenizeBorrowed(input)
		wantTokens := borrowed.Limit(maxBytes).Clone()
		wantIndices := append([]int(nil), borrowed.Limit(maxBytes).Indices()...)

		got := borrowed.Own().Limit(maxBytes)

		assert.Equal(t, len(wantTokens), got.Len(), "maxBytes=%d", maxBytes)
		if len(wantTokens) > 0 {
			assert.Equal(t, wantTokens, got.Tokens(), "maxBytes=%d", maxBytes)
			assert.Equal(t, wantIndices, got.Indices(), "maxBytes=%d", maxBytes)
		}
	}
}

func TestOwnedTokensConcurrentReads(t *testing.T) {
	owned := NewTokenizer(0).TokenizeBorrowed([]byte("2024-01-15 ERROR request failed")).Own()
	want := owned.Tokens()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < owned.Len(); i++ {
				_ = owned.At(i)
				_ = owned.Start(i)
			}
			_ = owned.Limit(10).Tokens()
			assert.Equal(t, want, owned.Tokens())
		}()
	}
	wg.Wait()
}
