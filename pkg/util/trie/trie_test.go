// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuffixTrieInsertAndGet(t *testing.T) {
	trie := NewSuffixTrie[string]()
	cid := "kubelet-kubepods-burstable-pod99dcb84d2a34f7e338778606703258c4.slice/cri-containerd-ec9ea0ad54dd0d96142d5dbe11eb3f1509e12ba9af739620c7b5ad377ce94602"
	cgroupPath := "/host/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/kubelet-kubepods-burstable-pod99dcb84d2a34f7e338778606703258c4.slice/cri-containerd-ec9ea0ad54dd0d96142d5dbe11eb3f1509e12ba9af739620c7b5ad377ce94602.scope"
	trie.Insert(cgroupPath, &cid)
	storedCid, ok := trie.Get(cgroupPath)
	assert.Equal(t, &cid, storedCid, "should return correct container id")
	assert.True(t, ok, "should return true if path is known")

	_, ok = trie.Get("unknown/path")
	assert.False(t, ok, "should return false if path is unknown")
}

func TestSuffixTrieDelete_EmptyKey(t *testing.T) {
	trie := NewSuffixTrie[string]()
	trie.Delete("")
	val := "value"
	trie.Insert("abc", &val)
	storedVal, ok := trie.Get("abc")
	assert.Equal(t, storedVal, &val, "Deleting empty key should allow to insert keys")
	assert.True(t, ok, "Deleting empty key should allow to insert keys")
}

func TestSuffixTrieDelete_NonExistentKey(t *testing.T) {
	trie := NewSuffixTrie[string]()
	val := "container123"
	trie.Insert("path/to/container", &val)
	trie.Delete("nonexistent/key")
	storedVal, ok := trie.Get("path/to/container")
	assert.True(t, ok, "Deleting nonexistent key should not affect existing keys")
	assert.Equal(t, &val, storedVal, "Deleting nonexistent key should not affect existing keys")
}

func TestSuffixTrieDelete_RootNode(t *testing.T) {
	trie := NewSuffixTrie[string]()
	val := "rootValue"
	trie.Insert("", &val)
	trie.Delete("")
	_, ok := trie.Get("")
	assert.False(t, ok, "Deleting root node should remove the value")
}

func TestSuffixTrieDelete_PartiallyMatchingKey(t *testing.T) {
	trie := NewSuffixTrie[string]()
	vals := []string{"value1", "value2", "value3"}
	trie.Insert("path/to/one", &vals[0])
	trie.Insert("path/to/one/two", &vals[1])
	trie.Insert("path/to/one/two/three", &vals[2])
	trie.Delete("path/to/one/two")
	storedVal1, ok1 := trie.Get("path/to/one")
	storedVal3, ok3 := trie.Get("path/to/one/two/three")
	assert.True(t, ok1, "Existing shorter keys should be unaffected")
	assert.True(t, ok3, "Existing longer keys should be unaffected")
	assert.Equal(t, &vals[0], storedVal1, "Existing shorter keys should be unaffected")
	assert.Equal(t, &vals[2], storedVal3, "Existing longer keys should be unaffected")
}
