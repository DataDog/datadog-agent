package common

import (
	"strconv"
	"strings"
)

type oidTrie struct {
	children map[int]*oidTrie
}

func newOidTrie() *oidTrie {
	return &oidTrie{}
}

func BuildTries(allOids []string) *oidTrie {
	root := newOidTrie()
	for _, oid := range allOids {
		current := root
		oid = strings.TrimLeft(oid, ".")
		segs := strings.Split(oid, ".")
		for _, seg := range segs {
			num, err := strconv.Atoi(seg)
			if err != nil {
				break
			}
			if current.children == nil {
				current.children = make(map[int]*oidTrie)
			}
			if _, ok := current.children[num]; !ok {
				current.children[num] = newOidTrie()
			}
			current = current.children[num]
		}
	}
	return root
}

func (o *oidTrie) exist(oid string, isLeaf bool) bool {
	current := o
	oid = strings.TrimLeft(oid, ".")
	segs := strings.Split(oid, ".")
	for _, seg := range segs {
		num, err := strconv.Atoi(seg)
		if err != nil {
			return false
		}

		child, ok := current.children[num]
		if !ok {
			return false
		}
		if len(child.children) == 0 {
			return true
		}
		current = child
	}
	if isLeaf {
		return false
	}
	return true
}

func (o *oidTrie) NodeExist(oid string) bool {
	return o.exist(oid, true)
}

func (o *oidTrie) LeafExist(oid string) bool {
	return o.exist(oid, false)
}
