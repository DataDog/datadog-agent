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
	assert.Equal(t, &OidTrie{
		Children: map[int]*OidTrie{
			1: {
				Children: map[int]*OidTrie{
					2: {
						Children: map[int]*OidTrie{
							3: {},
						},
					},
					3: {
						Children: map[int]*OidTrie{
							4: {
								Children: map[int]*OidTrie{
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
	assert.Equal(t, true, trie.LeafExist("1.2.3"))
	assert.Equal(t, true, trie.LeafExist("1.3.4.5"))
	assert.Equal(t, false, trie.LeafExist("1.2"))
	assert.Equal(t, false, trie.LeafExist("1.4"))
}

func Test_oidTrie_NodeExist(t *testing.T) {
	trie := BuildOidTrie([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, true, trie.NodeExist("1.2.3"))
	assert.Equal(t, true, trie.NodeExist("1.3.4.5"))
	assert.Equal(t, true, trie.NodeExist("1.2"))
	assert.Equal(t, false, trie.NodeExist("1.4"))
}

func Test_oidTrie_InvalidDigitIgnored(t *testing.T) {
	trie := BuildOidTrie([]string{"1.2.3", "1.3.9.A"})
	assert.Equal(t, true, trie.NodeExist("1.2.3"))
	assert.Equal(t, false, trie.NodeExist("1.4"))
}
