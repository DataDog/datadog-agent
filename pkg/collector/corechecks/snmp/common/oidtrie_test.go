package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildTries(t *testing.T) {
	trie := BuildTries([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, &oidTrie{
		children: map[int]*oidTrie{
			1: {
				children: map[int]*oidTrie{
					2: {
						children: map[int]*oidTrie{
							3: {},
						},
					},
					3: {
						children: map[int]*oidTrie{
							4: {
								children: map[int]*oidTrie{
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
	trie := BuildTries([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, true, trie.LeafExist("1.2.3"))
	assert.Equal(t, true, trie.LeafExist("1.3.4.5"))
	assert.Equal(t, true, trie.LeafExist("1.2"))
	assert.Equal(t, false, trie.LeafExist("1.4"))
}

func Test_oidTrie_NodeExist(t *testing.T) {
	trie := BuildTries([]string{"1.2.3", "1.3.4.5"})
	assert.Equal(t, true, trie.NodeExist("1.2.3"))
	assert.Equal(t, true, trie.NodeExist("1.3.4.5"))
	assert.Equal(t, false, trie.NodeExist("1.2"))
	assert.Equal(t, false, trie.NodeExist("1.4"))
}
