// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// Signature represents a structural signature of a TokenList
type Signature struct {
	Position string
	Length   int
	Hash     uint64
}

// NewSignature creates a signature from a TokenList
func NewSignature(tl *TokenList) Signature {
	if tl.IsEmpty() {
		return Signature{
			Position: "",
			Length:   0,
			Hash:     0,
		}
	}

	position := positionSignature(tl)

	// Include first word token value in signature if it exists
	// This prevents messages with different first words but similar signature from being in the same cluster
	// eg: I love burger vs You love burger
	if len(tl.Tokens) > 0 && tl.Tokens[0].Type == TokenWord {
		firstWordValue := tl.Tokens[0].Value
		position = firstWordValue + position
	}

	hash := computeHash(position)
	return Signature{
		Position: position,
		Length:   len(tl.Tokens),
		Hash:     hash,
	}
}

// Equals checks if two signatures are identical
func (s *Signature) Equals(other Signature) bool {
	return s.Position == other.Position &&
		s.Length == other.Length
}

// computeHash generates a hash for the signature
func computeHash(input string) uint64 {
	hash := fnv.New64a()
	hash.Write([]byte(input))
	return hash.Sum64()
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

// positionSignature generates position-based signature
func positionSignature(tl *TokenList) string {
	if tl.IsEmpty() {
		return ""
	}

	var positionParts []string
	for _, token := range tl.Tokens {
		positionParts = append(positionParts, token.Type.String())
	}
	return strings.Join(positionParts, "|")
}
