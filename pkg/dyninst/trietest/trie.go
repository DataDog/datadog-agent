// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package trietest is a test package for the critbit trie implementation
// used in the ebpf package.
package trietest

/*
#define CGO_TEST
#include "../ebpf/chased_pointers_trie.h"

const uint64_t max_size = (CPT_NUM_NODES);
*/
import "C"
import "fmt"

var maxSize = uint64(C.max_size)

const (
	insertResultExists  = 0
	insertResultSuccess = 1
	insertResultFull    = 2
	insertResultNull    = 3
	insertResultError   = 4
)

// key defines the composite key for the set.
type key struct {
	addr   uint64
	typeID uint32
}

// trieSet is a Go wrapper for the C crit-bit trieSet.
type trieSet struct {
	trie C.chased_pointers_trie_t
}

// newTrieSet creates and initializes a new Trie.
func newTrieSet() *trieSet {
	t := &trieSet{}
	C.chased_pointers_trie_init(&t.trie)
	return t
}

// Insert adds a key to the trie.
func (t *trieSet) insert(k key) (inserted, full bool) {
	ret := C.chased_pointers_trie_insert(&t.trie, C.uint64_t(k.addr), C.uint32_t(k.typeID))
	switch ret {
	case insertResultSuccess:
		return true, false
	case insertResultFull:
		return false, true
	case insertResultExists:
		return false, false
	case insertResultNull:
		panic("trie is null")
	case insertResultError:
		panic("trie is in an invalid state")
	default:
		panic(fmt.Sprintf("unreachable: %d", ret))
	}
}

func (t *trieSet) len() int {
	return int(C.chased_pointers_trie_len(&t.trie))
}

// Contains checks if a key exists in the trie.
func (t *trieSet) contains(k key) bool {
	ret := C.chased_pointers_trie_lookup(&t.trie, C.uint64_t(k.addr), C.uint32_t(k.typeID))
	return ret != 0
}

// Clear resets the trie.
func (t *trieSet) clear() {
	C.chased_pointers_trie_clear(&t.trie)
}
