package common

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
	"strings"
)

type OidTrie struct {
	Children map[int]*OidTrie
}

func newOidTrie() *OidTrie {
	return &OidTrie{}
}

// BuildTries builds the OidTrie from a list of OIDs
func BuildTries(allOids []string) *OidTrie {
	root := newOidTrie()
	for _, oid := range allOids {
		current := root
		oid = strings.TrimLeft(oid, ".")
		digits := strings.Split(oid, ".")
		for _, digit := range digits {
			num, err := strconv.Atoi(digit)
			if err != nil {
				break
			}
			if current.Children == nil {
				current.Children = make(map[int]*OidTrie)
			}
			if _, ok := current.Children[num]; !ok {
				current.Children[num] = newOidTrie()
			}
			current = current.Children[num]
		}
	}
	return root
}

func (o *OidTrie) exist(oid string, isLeaf bool) bool {
	current := o
	oid = strings.TrimLeft(oid, ".")
	digits := strings.Split(oid, ".")
	for _, digit := range digits {
		num, err := strconv.Atoi(digit)
		if err != nil {
			return false
		}

		child, ok := current.Children[num]
		if !ok {
			return false
		}
		if len(child.Children) == 0 {
			return true
		}
		current = child
	}
	if isLeaf {
		return false
	}
	return true
}

// NodeExist checks if the oid is a known node
func (o *OidTrie) NodeExist(oid string) bool {
	return o.exist(oid, false)
}

// LeafExist checks if the oid is a known leaf
func (o *OidTrie) LeafExist(oid string) bool {
	return o.exist(oid, true)
}

// DebugPrint is used to print the whole Trie for debugging purpose
func (o *OidTrie) DebugPrint() {
	o.debugPrintRecursive("")
}

// debugPrintRecursive is used to print the whole Trie for debugging purpose
func (o *OidTrie) debugPrintRecursive(prefix string) {
	log.Infof("Print OidTrie")
	if len(o.Children) == 0 {
		log.Infof("OID: %s", prefix)
	}
	for oid, child := range o.Children {
		child.debugPrintRecursive(fmt.Sprintf("%s.%d", prefix, oid))
	}
}
