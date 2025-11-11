// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// Merge merges this node with that and returns the merged result
func (n *innerNode) Merge(that InnerNode) (InnerNode, error) {
	newChildren := map[string]Node{}

	// iterate our keys
	for _, name := range n.ChildrenKeys() {
		ourChild, _ := n.GetChild(name)

		if !that.HasChild(name) {
			// if their tree doesn't have the node, use our node
			newChildren[name] = ourChild
			continue
		}
		theirChild, _ := that.GetChild(name)
		ourLeaf, ourIsLeaf := ourChild.(LeafNode)
		theirLeaf, theirIsLeaf := theirChild.(LeafNode)

		// If subtree shapes differ, take the longer branch
		// TODO: Improve error handling in a follow-up PR. We should collect errors
		// and log.Error them, but also display these errors in more places
		if ourIsLeaf && !theirIsLeaf {
			newChildren[name] = theirChild
			continue
		} else if !ourIsLeaf && theirIsLeaf {
			newChildren[name] = ourChild
			continue
		}

		if ourIsLeaf && theirIsLeaf {
			// both are leafs, check the priority by source
			if ourLeaf.Source() == theirLeaf.Source() || theirLeaf.SourceGreaterThan(ourLeaf.Source()) {
				newChildren[name] = theirChild
			} else {
				newChildren[name] = ourChild
			}
			continue
		}

		// both are inner nodes, recursively merge
		ourInner := ourChild.(InnerNode)
		theirInner := theirChild.(InnerNode)
		result, err := ourInner.Merge(theirInner)
		if err != nil {
			log.Errorf("merging config tree: %v\n", err)
			continue
		}
		newChildren[name] = result
	}

	// iterate their keys
	for _, name := range that.ChildrenKeys() {
		if !n.HasChild(name) {
			newChildren[name], _ = that.GetChild(name)
		}
	}

	return newInnerNode(newChildren), nil
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
//
// This method should only be called on the root of a tree, not on an inner node with parents.
// TODO: Consider adding a RootNode type, move this method to that.
func (n *innerNode) SetAt(key []string, value interface{}, source model.Source) error {
	if len(key) == 0 {
		return errors.New("empty key given to Set")
	}
	newNode, err := setNodeAtPath(n, key, value, source)
	if inner, ok := newNode.(*innerNode); ok {
		n.children = inner.children
	}
	return err
}

// setNodeAtPath allocates a new branch, ending in a leaf at the given path of fields, with the
// given value, and returns the root of that branch. If a leaf already exists at that path,
// instead it is modified and no branch is allocated and this returns nil
func setNodeAtPath(n *innerNode, fields []string, value interface{}, source model.Source) (Node, error) {
	if len(fields) == 0 {
		return newLeafNode(value, source), nil
	}
	f := fields[0]

	// Locate the next node down in the tree (or nil if it doesn't exist)
	var next *innerNode
	if n != nil {
		if child, _ := n.GetChild(f); child != nil {
			if innerNode, ok := child.(*innerNode); ok {
				next = innerNode
			} else if leafNode, ok := child.(LeafNode); ok {
				// If we find a leaf, simply replace its value, and return nil for
				// the first return value because no node was created
				return nil, leafNode.ReplaceValue(value)
			}
		}
	}

	// Recursively set the node at the remaining part of the path
	createdNode, err := setNodeAtPath(next, fields[1:], value, source)
	if err != nil || createdNode == nil {
		return nil, err
	}

	// Create a new inner node using the modified list of child nodes
	var copyChildren map[string]Node
	if n != nil {
		copyChildren = maps.Clone(n.children)
	} else {
		copyChildren = make(map[string]Node)
	}
	copyChildren[f] = createdNode
	return newInnerNode(copyChildren), nil
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

		if childInner, ok := child.(InnerNode); ok {
			childDump := childInner.DumpSettings(selector)
			if len(childDump) != 0 {
				res[k] = childDump
			}
		}
	}
	return res
}
