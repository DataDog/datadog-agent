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

func TestSuffixTrieInsertAndGet_EmptyKey(t *testing.T) {
	trie := NewSuffixTrie[string]()
	cid := "kubepods-podea2e69b4_0bd4_48f0_b75f_cc7c79111027.slice/cri-containerd-7ee34913fbefa6a0dbbdf1bbc302cc29a1bde3b83b8b9f2252a4d4360c373848.scope"
	cgp := "kubepods-podea2e69b4_0bd4_48f0_b75f_cc7c79111027.slice/cri-containerd-7ee34913fbefa6a0dbbdf1bbc302cc29a1bde3b83b8b9f2252a4d4360c373848.scope"
	cgroupPath := "/host/sys/fs/cgroup/cpuset/kubepods.slice/kubepods-podea2e69b4_0bd4_48f0_b75f_cc7c79111027.slice/cri-containerd-7ee34913fbefa6a0dbbdf1bbc302cc29a1bde3b83b8b9f2252a4d4360c373848.scope"
	cid2 := "kubepods-pod5a9426d5_65f3_45cb_b8e3_ea5f687030ef.slice/cri-containerd-b07c21533237150ce3fa84817e79d6def7c742c840cd3982881a37b593446fdc.scope"
	cgp2 := "kubepods-pod5a9426d5_65f3_45cb_b8e3_ea5f687030ef.slice/cri-containerd-b07c21533237150ce3fa84817e79d6def7c742c840cd3982881a37b593446fdc.scope"
	cgroupPath2 := "/host/sys/fs/cgroup/cpuset/kubepods.slice/kubepods-pod5a9426d5_65f3_45cb_b8e3_ea5f687030ef.slice/cri-containerd-b07c21533237150ce3fa84817e79d6def7c742c840cd3982881a37b593446fdc.scope"
	trie.Insert(cgp, &cid)
	trie.Insert(cgp2, &cid2)
	trie.Insert("", &cid)
	storedCid, ok := trie.Get(cgroupPath)
	storedCid2, ok2 := trie.Get(cgroupPath2)
	assert.Equal(t, &cid, storedCid, "should return correct container id")
	assert.Equal(t, &cid2, storedCid2, "should return correct container id (second)")
	assert.True(t, ok, "should return true if path is known")
	assert.True(t, ok2, "should return true if path is known")

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
