// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildTries(t *testing.T) {
	trie := BuildOidTrie([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, &OIDTrie{
		Children: map[int]*OIDTrie{
			1: {
				Children: map[int]*OIDTrie{
					2: {
						Children: map[int]*OIDTrie{
							3: {},
						},
					},
					3: {
						Children: map[int]*OIDTrie{
							4: {
								Children: map[int]*OIDTrie{
									5: {},
								},
							},
						},
					},
				},
			},
		},
	}, trie)
}

func Test_oidTrie_LeafExist(t *testing.T) {
	trie := BuildOidTrie([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, false, trie.LeafExist(""))
	assert.Equal(t, true, trie.LeafExist("1.2.3"))
	assert.Equal(t, true, trie.LeafExist("1.3.4.5"))
	assert.Equal(t, false, trie.LeafExist("1.2"))
	assert.Equal(t, false, trie.LeafExist("1.4"))
	assert.Equal(t, false, trie.LeafExist("1"))
}

func Test_oidTrie_NodeExist(t *testing.T) {
	trie := BuildOidTrie([]string{"1", "1.2.3", "1.3.4.5"})
	assert.Equal(t, false, trie.NonLeafNodeExist(""))
	assert.Equal(t, true, trie.NonLeafNodeExist("1.2"))
	assert.Equal(t, false, trie.NonLeafNodeExist("1.2.3")) // this is a leaf, not a node
	assert.Equal(t, true, trie.NonLeafNodeExist("1.3.4"))
	assert.Equal(t, false, trie.NonLeafNodeExist("1.3.4.5"))
	assert.Equal(t, true, trie.NonLeafNodeExist("1.2"))
	assert.Equal(t, false, trie.NonLeafNodeExist("1.4"))
}

func Test_oidTrie_InvalidDigitIgnored(t *testing.T) {
	trie := BuildOidTrie([]string{"1.2.3", "1.3.9.A"})
	assert.Equal(t, true, trie.NonLeafNodeExist("1.2"))
	assert.Equal(t, true, trie.LeafExist("1.2.3"))
	assert.Equal(t, false, trie.NonLeafNodeExist("1.3"))
	assert.Equal(t, false, trie.NonLeafNodeExist("1.4"))
}
