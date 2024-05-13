// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type trieNode struct {
	children map[rune]*trieNode
	cid      string
}

func newTrieNode() *trieNode {
	return &trieNode{
		children: make(map[rune]*trieNode),
	}
}

type suffixTrie struct {
	root *trieNode
}

func newSuffixTrie() *suffixTrie {
	return &suffixTrie{
		root: newTrieNode(),
	}
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Delete deletes a suffix from the suffixTrie
func (t *suffixTrie) Delete(suffix string) {
	reversedSuffix := reverse(suffix)
	t.delete(t.root, reversedSuffix, 0)
}

func (t *suffixTrie) delete(node *trieNode, suffix string, depth int) bool {
	if node == nil {
		return false
	}

	if len(suffix) == depth {
		node.cid = ""
		return len(node.children) == 0
	}

	char := rune(suffix[depth])
	if nextNode, ok := node.children[char]; ok {
		if t.delete(nextNode, suffix, depth+1) {
			delete(node.children, char)
			return len(node.children) == 0 && node.cid == ""
		}
	}
	return false
}

// Insert adds a new suffix to the trie
func (t *suffixTrie) Insert(suffix, cid string) {
	reversedSuffix := reverse(suffix)
	node := t.root

	for _, char := range reversedSuffix {
		if _, ok := node.children[char]; !ok {
			node.children[char] = newTrieNode()
		}
		node = node.children[char]
	}

	node.cid = cid
}

// Get returns the container id for a given suffix
func (t *suffixTrie) Get(text string) string {
	reversedText := reverse(text)
	node := t.root

	for _, char := range reversedText {
		if node.cid != "" {
			return node.cid
		}
		if next, ok := node.children[char]; ok {
			node = next
		} else {
			return ""
		}
	}

	return node.cid
}

type containerFilter struct {
	wlm workloadmeta.Component

	mutex sync.RWMutex
	trie  *suffixTrie
}

// newContainerFilter returns a new container filter
func newContainerFilter(wlm workloadmeta.Component) *containerFilter {
	cf := &containerFilter{
		trie: newSuffixTrie(),
		wlm:  wlm,
	}
	return cf
}

func (cf *containerFilter) start() {
	if cf.wlm == nil {
		return
	}
	evBundle := cf.wlm.Subscribe("cid-mapper", workloadmeta.NormalPriority, workloadmeta.NewFilter(
		&workloadmeta.FilterParams{
			Kinds:     []workloadmeta.Kind{workloadmeta.KindContainer},
			Source:    workloadmeta.SourceAll,
			EventType: workloadmeta.EventTypeAll,
		},
	))
	defer cf.wlm.Unsubscribe(evBundle)
	for evs := range evBundle {
		evs.Acknowledge()
		cf.mutex.Lock()
		for _, ev := range evs.Events {
			cf.handleEvent(ev)
		}
		cf.mutex.Unlock()
	}
}

func (cf *containerFilter) handleEvent(ev workloadmeta.Event) {
	cont, ok := ev.Entity.(*workloadmeta.Container)
	if !ok {
		log.Errorf("unexpected event type: %T", ev)
		return
	}
	switch ev.Type {
	case workloadmeta.EventTypeSet:
		if cont.CgroupPath != "" {
			cf.trie.Insert(cont.CgroupPath, cont.ID)
		}
	case workloadmeta.EventTypeUnset:
		cf.trie.Delete(cont.CgroupPath)
	default:
		log.Errorf("unexpected event type: %v", ev.Type)
	}
}

// ContainerFilter returns a filter that will match cgroup folders containing a container id
func (cf *containerFilter) ContainerFilter(path, name string) (string, error) {
	if res, _ := cgroups.ContainerFilter(path, name); res != "" {
		return res, nil
	}
	cf.mutex.RLock()
	res := cf.trie.Get(path)
	cf.mutex.RUnlock()
	return res, nil
}
