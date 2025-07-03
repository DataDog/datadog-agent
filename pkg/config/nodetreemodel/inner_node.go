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
	children map[string]Node
}

func newInnerNode(children map[string]Node) *innerNode {
	contents := make(map[string]Node, len(children))
	for k, v := range children {
		contents[strings.ToLower(k)] = v
	}
	node := &innerNode{children: contents}
	return node
}

var _ Node = (*innerNode)(nil)

// Clone clones a InnerNode and all its children
func (n *innerNode) Clone() Node {
	children := make(map[string]Node)
	for k, node := range n.children {
		children[k] = node.Clone()
	}
	return newInnerNode(children)
}

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *innerNode) GetChild(key string) (Node, error) {
	mkey := strings.ToLower(key)
	child, found := n.children[mkey]
	if !found {
		return nil, ErrNotFound
	}
	return child, nil
}

// HasChild returns true if the node has a child for that given key
func (n *innerNode) HasChild(key string) bool {
	_, ok := n.children[strings.ToLower(key)]
	return ok
}

// Merge mergs src node within current tree
func (n *innerNode) Merge(src InnerNode) error {
	for _, name := range src.ChildrenKeys() {
		srcChild, _ := src.GetChild(name)

		if !n.HasChild(name) {
			n.children[name] = srcChild.Clone()
		} else {
			// We alredy have child with the same name

			dstChild, _ := n.GetChild(name)

			dstLeaf, dstIsLeaf := dstChild.(LeafNode)
			srcLeaf, srcIsLeaf := srcChild.(LeafNode)
			if srcIsLeaf != dstIsLeaf {
				// Ignore the source (incoming) node and keep the destination (current) node
				// TODO: Improve error handling in a follow-up PR. We should collect errors
				// and log.Error them, but not break functionality in doing so
				continue
			}

			if srcIsLeaf {
				if srcLeaf.Source() == dstLeaf.Source() || srcLeaf.SourceGreaterThan(dstLeaf.Source()) {
					n.children[name] = srcLeaf.Clone()
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
	mapkeys := maps.Keys(n.children)
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

	part := key[0]
	if keyLen == 1 {
		node, ok := n.children[part]
		if !ok {
			n.children[part] = newLeafNode(value, source)
			return true, nil
		}

		if leaf, ok := node.(LeafNode); ok {
			if source == leaf.Source() || source.IsGreaterThan(leaf.Source()) {
				n.children[part] = newLeafNode(value, source)
				return true, nil
			}
			return false, nil
		}
		return false, fmt.Errorf("can't overrides inner node with a leaf node")
	}

	// new node case
	if _, ok := n.children[part]; !ok {
		newNode := newInnerNode(nil)
		n.children[part] = newNode
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
	n.children[name] = node
}

// RemoveChild removes a node from the current node
func (n *innerNode) RemoveChild(name string) {
	delete(n.children, name)
}

// DumpSettings clone the entire tree starting from the node into a map based on the leaf source.
//
// The selector will be call with the source of each leaf to determine if it should be included in the dump.
func (n *innerNode) DumpSettings(selector func(model.Source) bool) map[string]interface{} {
	res := map[string]interface{}{}

	for _, k := range n.ChildrenKeys() {
		child, _ := n.GetChild(k)
		if leaf, ok := child.(LeafNode); ok {
			if selector(leaf.Source()) {
				res[k] = leaf.Get()
			}
			continue
		}

		childDump := child.(InnerNode).DumpSettings(selector)
		if len(childDump) != 0 {
			res[k] = childDump
		}
	}
	return res
}
