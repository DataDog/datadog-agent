// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// Signature represents a structural signature of a TokenList
type Signature struct {
	Position string
	Count    string
	Length   int
	Hash     uint64
}

// NewSignature creates a signature from a TokenList
func NewSignature(tl *TokenList) Signature {
	if tl.IsEmpty() {
		return Signature{
			Position: "",
			Count:    "",
			Length:   0,
			Hash:     0,
		}
	}

	position := positionSignature(tl)
	count := countSignature(tl)

	combined := fmt.Sprintf("%s|%s", position, count)
	hash := computeHash(combined)

	return Signature{
		Position: position,
		Count:    count,
		Length:   len(tl.Tokens),
		Hash:     hash,
	}
}

// Equals checks if two signatures are identical
func (s *Signature) Equals(other Signature) bool {
	return s.Hash == other.Hash
}

// computeHash generates a hash for the signature
func computeHash(input string) uint64 {
	hash := fnv.New64a()
	hash.Write([]byte(input))
	return hash.Sum64()
}

// String returns a string representation of the signature
func (s *Signature) String() string {
	return fmt.Sprintf("Sig{pos:%s, count:%s, len:%d, hash:%x}",
		s.Position, s.Count, s.Length, s.Hash)
}

// IsEmpty returns true if the signature represents an empty TokenList
func (s *Signature) IsEmpty() bool {
	return s.Length == 0
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

// countSignature generates count-based signature
func countSignature(tl *TokenList) string {
	if tl.IsEmpty() {
		return ""
	}

	typeCounts := make(map[TokenType]int)
	for _, token := range tl.Tokens {
		typeCounts[token.Type]++
	}

	var countParts []string
	for tokenType, count := range typeCounts {
		countParts = append(countParts, fmt.Sprintf("%s:%d", tokenType.String(), count))
	}

	// Sort to ensure deterministic signature
	sort.Strings(countParts)
	return strings.Join(countParts, ";")
}
