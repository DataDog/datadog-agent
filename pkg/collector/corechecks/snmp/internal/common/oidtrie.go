// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OIDTrie is a trie structure that represent OIDs as tree
// It's an efficient data structure for verifying if an OID is known or not.
// The search complexity of NonLeafNodeExist / LeafExist methods are O(n),
// where n is the length of the OID (number of dot separated numbers).
// The search complexity doesn't depend on the size of the trie.
type OIDTrie struct {
	Children map[int]*OIDTrie
}

func newOidTrie() *OIDTrie {
	return &OIDTrie{}
}

// BuildOidTrie builds the OIDTrie from a list of OIDs
func BuildOidTrie(allOIDs []string) *OIDTrie {
	root := newOidTrie()
	for _, oid := range allOIDs {
		current := root

		numbers, err := oidToNumbers(oid)
		if err != nil {
			log.Debugf("error processing oid `%s`: %s", oid, err)
			continue
		}

		for _, num := range numbers {
			if current.Children == nil {
				current.Children = make(map[int]*OIDTrie)
			}
			if _, ok := current.Children[num]; !ok {
				current.Children[num] = newOidTrie()
			}
			current = current.Children[num]
		}
	}
	return root
}

func oidToNumbers(oid string) ([]int, error) {
	oid = strings.TrimLeft(oid, ".")
	strNumbers := strings.Split(oid, ".")
	var numbers []int
	for _, strNumber := range strNumbers {
		num, err := strconv.Atoi(strNumber)
		if err != nil {
			return nil, fmt.Errorf("error converting digit %s (oid=%s)", strNumber, oid)
		}
		numbers = append(numbers, num)
	}
	return numbers, nil
}

func (o *OIDTrie) getNode(oid string) (*OIDTrie, error) {
	if oid == "" {
		return nil, fmt.Errorf("invalid empty OID")
	}
	current := o
	oid = strings.TrimLeft(oid, ".")
	digits := strings.Split(oid, ".")
	for _, digit := range digits {
		num, err := strconv.Atoi(digit)
		if err != nil {
			return nil, fmt.Errorf("invalid OID: %s", err)
		}
		child, ok := current.Children[num]
		if !ok {
			return nil, fmt.Errorf("node `%s` not found in OIDTrie", oid)
		}
		current = child
	}
	return current, nil
}

// NonLeafNodeExist checks if the oid is a known node (a node have at least one child)
func (o *OIDTrie) NonLeafNodeExist(oid string) bool {
	node, err := o.getNode(oid)
	if err != nil {
		return false
	}
	return len(node.Children) > 0
}

// LeafExist checks if the oid is a known leaf
func (o *OIDTrie) LeafExist(oid string) bool {
	node, err := o.getNode(oid)
	if err != nil {
		return false
	}
	return len(node.Children) == 0
}

// DebugPrint is used to print the whole Trie for debugging purpose
func (o *OIDTrie) DebugPrint() {
	o.debugPrintRecursive("")
}

// debugPrintRecursive is used to print the whole Trie for debugging purpose
func (o *OIDTrie) debugPrintRecursive(prefix string) {
	log.Infof("Print OIDTrie")
	if len(o.Children) == 0 {
		log.Infof("OID: %s", prefix)
	}
	for oid, child := range o.Children {
		child.debugPrintRecursive(fmt.Sprintf("%s.%d", prefix, oid))
	}
}
