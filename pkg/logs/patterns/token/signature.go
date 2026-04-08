// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"fmt"
	"hash"
	"hash/fnv"
	"strings"
	"sync"
	"unsafe"
)

// Signature represents a structural signature of a TokenList
type Signature struct {
	Position string
	Length   int
	Hash     uint64
}

// NewSignature creates a signature from a TokenList.
// Uses a single strings.Builder pass to avoid the []string + Join + string-concat allocations
// that the old positionSignature helper required (3 allocs → 2 allocs per call).
func NewSignature(tl *TokenList) Signature {
	n := len(tl.Tokens)
	if n == 0 {
		return Signature{
			Position: "",
			Length:   0,
			Hash:     0,
		}
	}

	// Single-pass build: write optional first-word-value prefix then token type names.
	// Preserves the original format: firstWordValue is concatenated directly onto the
	// position string with no separator (e.g. "helloWord|Whitespace|Word").
	var sb strings.Builder
	if tl.Tokens[0].Type == TokenWord {
		sb.WriteString(tl.Tokens[0].Value)
	}
	for i, tok := range tl.Tokens {
		if i > 0 {
			sb.WriteByte('|')
		}
		sb.WriteString(tokenTypeNames[tok.Type])
	}
	position := sb.String()

	hash := computeHash(position)
	return Signature{
		Position: position,
		Length:   n,
		Hash:     hash,
	}
}

// Equals checks if two signatures are identical
func (s *Signature) Equals(other Signature) bool {
	return s.Position == other.Position &&
		s.Length == other.Length
}

var fnvPool = sync.Pool{
	New: func() any { return fnv.New64a() },
}

// computeHash generates a hash for the signature.
// Uses FNV-1a (not maphash) for deterministic, reproducible hashes
// that remain stable across process restarts.
func computeHash(input string) uint64 {
	h := fnvPool.Get().(hash.Hash64)
	h.Reset()
	if len(input) > 0 {
		h.Write(unsafe.Slice(unsafe.StringData(input), len(input)))
	}
	v := h.Sum64()
	fnvPool.Put(h)
	return v
}

// String returns a string representation of the signature
func (s *Signature) String() string {
	return fmt.Sprintf("Sig{pos:%s, len:%d, hash:%x}",
		s.Position, s.Length, s.Hash)
}

// IsEmpty returns true if the signature represents an empty TokenList
func (s *Signature) IsEmpty() bool {
	return s.Length == 0
}

// HasSameStructure checks if two signatures have the same positional structure
func (s *Signature) HasSameStructure(other Signature) bool {
	return s.Position == other.Position && s.Length == other.Length
}

// GetHashBucket returns the hash bucket for efficient clustering
func (s *Signature) GetHashBucket() uint64 {
	return s.Hash
}
