// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"golang.org/x/exp/maps"
)

// innerNode represents an non-leaf node of the config
type innerNode struct {
	val map[string]Node
	// remapCase maps each lower-case key to the original case. This
	// enables GetChild to retrieve values using case-insensitive keys
	remapCase map[string]string
}

func newInnerNodeImpl() *innerNode {
	return &innerNode{val: map[string]Node{}, remapCase: map[string]string{}}
}

func newInnerNodeImplWithData(v map[string]interface{}, source model.Source) (*innerNode, error) {
	children := map[string]Node{}
	for name, value := range v {
		n, err := NewNode(value, source)
		if err != nil {
			return nil, err
		}
		children[name] = n
	}
	in := &innerNode{val: children}
	in.makeRemapCase()
	return in, nil
}

var _ Node = (*innerNode)(nil)

// Clone clones a InnerNode and all its children
func (n *innerNode) Clone() Node {
	clone := newInnerNodeImpl()

	for k, node := range n.val {
		clone.val[k] = node.Clone()
	}
	clone.makeRemapCase()
	return clone
}

// makeRemapCase creates a map that converts keys from their lower-cased version to their original case
func (n *innerNode) makeRemapCase() {
	remap := make(map[string]string)
	for k := range n.val {
		remap[strings.ToLower(k)] = k
	}
	n.remapCase = remap
}

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *innerNode) GetChild(key string) (Node, error) {
	mkey := n.remapCase[strings.ToLower(key)]
	child, found := n.val[mkey]
	if !found {
		return nil, ErrNotFound
	}
	return child, nil
}

// HasChild returns true if the node has a child for that given key
func (n *innerNode) HasChild(key string) bool {
	_, ok := n.val[key]
	return ok
}

// Merge mergs src node within current tree
func (n *innerNode) Merge(src InnerNode) error {
	defer n.makeRemapCase()

	for _, name := range src.ChildrenKeys() {
		srcChild, _ := src.GetChild(name)

		if !n.HasChild(name) {
			n.val[name] = srcChild.Clone()
		} else {
			// We alredy have child with the same name

			dstChild, _ := n.GetChild(name)

			dstLeaf, dstIsLeaf := dstChild.(LeafNode)
			srcLeaf, srcIsLeaf := srcChild.(LeafNode)
			if srcIsLeaf != dstIsLeaf {
				return fmt.Errorf("tree conflict, can't merge inner and leaf nodes for '%s'", name)
			}

			if srcIsLeaf {
				if srcLeaf.SourceGreaterOrEqual(dstLeaf.Source()) {
					n.val[name] = srcLeaf.Clone()
				}
			} else {
				dstInner, _ := dstChild.(InnerNode)
				childInner, _ := srcChild.(InnerNode)
				if err := dstInner.Merge(childInner); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *innerNode) ChildrenKeys() []string {
	mapkeys := maps.Keys(n.val)
	// map keys are iterated non-deterministically, sort them
	slices.Sort(mapkeys)
	return mapkeys
}

// SetAt sets a value in the tree by either creating a leaf node or updating one if the priority is equal or higher than
// the existing one. The function returns true if an update was done or false if nothing was changed.
//
// The key parts should already be lowercased.
func (n *innerNode) SetAt(key []string, value interface{}, source model.Source) (bool, error) {
	keyLen := len(key)
	if keyLen == 0 {
		return false, fmt.Errorf("empty key given to Set")
	}

	defer n.makeRemapCase()

	part := key[0]
	if keyLen == 1 {
		node, ok := n.val[part]
		if !ok {
			n.val[part] = newLeafNodeImpl(value, source)
			return true, nil
		}

		if leaf, ok := node.(LeafNode); ok {
			if leaf.Source().IsGreaterOrEqualThan(source) {
				n.val[part] = newLeafNodeImpl(value, source)
				return true, nil
			}
			return false, nil
		}
		return false, fmt.Errorf("can't overrides inner node with a leaf node")
	}

	// new node case
	if _, ok := n.val[part]; !ok {
		newNode := newInnerNodeImpl()
		n.val[part] = newNode
		return newNode.SetAt(key[1:keyLen], value, source)
	}

	// update node case
	child, err := n.GetChild(part)
	node, ok := child.(InnerNode)
	if err != nil || !ok {
		return false, fmt.Errorf("can't update a leaf node into a inner node")
	}
	return node.SetAt(key[1:keyLen], value, source)
}

// InsertChildNode sets a node in the current node
func (n *innerNode) InsertChildNode(name string, node Node) {
	n.val[name] = node
	n.makeRemapCase()
}
