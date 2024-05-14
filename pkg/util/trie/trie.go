// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package trie provides a SuffixTrie data structure
// that can be used to index data by suffixes of strings.
package trie

// node represents a node in the trie
type node[T any] struct {
	children map[rune]*node[T]
	value    *T
}

// newTrieNode creates a new node
func newTrieNode[T any]() *node[T] {
	return &node[T]{
		children: make(map[rune]*node[T]),
	}
}

// SuffixTrie represents a trie data structure for suffixes
type SuffixTrie[T any] struct {
	root *node[T]
}

// NewSuffixTrie creates a new SuffixTrie
func NewSuffixTrie[T any]() *SuffixTrie[T] {
	return &SuffixTrie[T]{
		root: newTrieNode[T](),
	}
}

// reverse reverses a string
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Delete deletes a suffix from the SuffixTrie
func (t *SuffixTrie[T]) Delete(suffix string) {
	reversedSuffix := reverse(suffix)
	t.delete(t.root, reversedSuffix, 0)
}

// delete deletes a suffix from the SuffixTrie
func (t *SuffixTrie[T]) delete(node *node[T], suffix string, depth int) bool {
	if node == nil {
		return false
	}

	if len(suffix) == depth {
		node.value = nil
		return len(node.children) == 0
	}

	char := rune(suffix[depth])
	if nextNode, ok := node.children[char]; ok {
		if t.delete(nextNode, suffix, depth+1) {
			delete(node.children, char)
			return len(node.children) == 0 && node.value == nil
		}
	}
	return false
}

// Insert stores the value for a given suffix
func (t *SuffixTrie[T]) Insert(suffix string, value *T) {
	reversedSuffix := reverse(suffix)
	n := t.root

	for _, char := range reversedSuffix {
		if _, ok := n.children[char]; !ok {
			n.children[char] = newTrieNode[T]()
		}
		n = n.children[char]
	}

	n.value = value
}

// Get returns the value for the first suffix that matches the given key
// Example: if the trie contains the suffixes "foo" and "foobar" and the key is "foobarbaz",
// the value for "foo" will be returned
func (t *SuffixTrie[T]) Get(key string) (*T, bool) {
	reversedText := reverse(key)
	n := t.root

	for _, char := range reversedText {
		if n.value != nil {
			return n.value, true
		}
		if next, ok := n.children[char]; ok {
			n = next
		} else {
			return nil, false
		}
	}

	if n.value == nil {
		return nil, false
	}

	return n.value, true
}
