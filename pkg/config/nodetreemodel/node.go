// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = errors.New("not found")

// missingLeafImpl is a none-object representing when a child node is missing
var missingLeaf = &nodeImpl{source: model.SourceUnknown}

// Node is a inner or leaf node in the config tree
type Node interface {
	IsLeafNode() bool
	IsInnerNode() bool
	Get() interface{}
	ChildrenKeys() []string
}

type nodeImpl struct {
	children map[string]*nodeImpl
	val      interface{}
	source   model.Source
}

var _ Node = (*nodeImpl)(nil)

func newInnerNode(children map[string]*nodeImpl) *nodeImpl {
	if children == nil {
		children = map[string]*nodeImpl{}
	}
	return &nodeImpl{children: children}
}

func newLeafNode(v interface{}, source model.Source) *nodeImpl {
	return &nodeImpl{val: v, source: source}
}

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *nodeImpl) GetChild(key string) (*nodeImpl, error) {
	if n.IsLeafNode() {
		return nil, errors.New("cannot GetChild of leaf node")
	}
	mkey := strings.ToLower(key)
	child, found := n.children[mkey]
	if !found {
		return nil, ErrNotFound
	}
	return child, nil
}

// HasChild returns true if the node has a child for that given key
func (n *nodeImpl) HasChild(key string) bool {
	if n.IsLeafNode() {
		return false
	}
	_, ok := n.children[strings.ToLower(key)]
	return ok
}

// Merge merges this node with that and returns the merged result
func (n *nodeImpl) Merge(that *nodeImpl) (*nodeImpl, error) {
	if n.IsLeafNode() {
		return nil, errors.New("cannot Merge into a leaf node")
	}
	if that.IsLeafNode() {
		return nil, errors.New("cannot Merge with a leaf node")
	}

	newChildren := map[string]*nodeImpl{}

	// iterate our keys
	for _, name := range n.ChildrenKeys() {
		ourChild, _ := n.GetChild(name)

		if !that.HasChild(name) {
			// if their tree doesn't have the node, use our node
			newChildren[name] = ourChild
			continue
		}
		theirChild, _ := that.GetChild(name)
		ourIsLeaf := ourChild.IsLeafNode()
		theirIsLeaf := theirChild.IsLeafNode()

		// If subtree shapes differ, take their branch, unless it is an empty leaf
		if ourIsLeaf != theirIsLeaf {
			if theirChild.IsLeafNode() && theirChild.Get() == nil {
				newChildren[name] = ourChild
				continue
			}
			newChildren[name] = theirChild
			continue
		}

		if ourIsLeaf && theirIsLeaf {
			// both are leafs, check the priority by source
			if ourChild.Source() == theirChild.Source() || theirChild.SourceGreaterThan(ourChild.Source()) {
				newChildren[name] = theirChild
			} else {
				newChildren[name] = ourChild
			}
			continue
		}

		// both are inner nodes, recursively merge
		result, err := ourChild.Merge(theirChild)
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
func (n *nodeImpl) ChildrenKeys() []string {
	mapkeys := slices.Collect(maps.Keys(n.children))
	// map keys are iterated non-deterministically, sort them
	slices.Sort(mapkeys)
	return mapkeys
}

// setAt sets a value in the tree by either creating a leaf node or updating one if the priority is equal or higher than
// the existing one. The function returns true if an update was done or false if nothing was changed.
//
// The key parts should already be lowercased.
//
// This method should only be called on the root of a tree, not on an inner node with parents.
func (n *nodeImpl) setAt(key []string, value interface{}, source model.Source) error {
	if len(key) == 0 {
		return errors.New("empty key given to Set")
	}
	newNode, err := setNodeAtPath(n, key, value, source)
	if newNode != nil && newNode.IsInnerNode() {
		n.children = newNode.children
	}
	return err
}

// setNodeAtPath allocates a new branch, ending in a leaf at the given path of fields, with the
// given value, and returns the root of that branch. If a leaf already exists at that path,
// instead it is modified and no branch is allocated and this returns nil
func setNodeAtPath(n *nodeImpl, fields []string, value interface{}, source model.Source) (*nodeImpl, error) {
	if len(fields) == 0 {
		return newLeafNode(value, source), nil
	}
	f := fields[0]

	// Locate the next node down in the tree (or nil if it doesn't exist)
	var next *nodeImpl
	if n != nil {
		if child, _ := n.GetChild(f); child != nil {
			if child.IsInnerNode() {
				next = child
			} else {
				// If we find a leaf, simply replace its value, and return nil for
				// the first return value because no node was created
				return nil, child.ReplaceValue(value)
			}
		}
	}

	// Recursively set the node at the remaining part of the path
	createdNode, err := setNodeAtPath(next, fields[1:], value, source)
	if err != nil || createdNode == nil {
		return nil, err
	}

	// Create a new inner node using the modified list of child nodes
	var copyChildren map[string]*nodeImpl
	if n != nil {
		copyChildren = maps.Clone(n.children)
	} else {
		copyChildren = make(map[string]*nodeImpl)
	}
	copyChildren[f] = createdNode
	return newInnerNode(copyChildren), nil
}

// InsertChildNode sets a node in the current node
func (n *nodeImpl) InsertChildNode(name string, node *nodeImpl) {
	if n.IsLeafNode() {
		log.Error("cannot InsertChildNode of leaf node")
		return
	}
	n.children[name] = node
}

// RemoveChild removes a node from the current node
func (n *nodeImpl) RemoveChild(name string) {
	if n.IsLeafNode() {
		log.Error("cannot RemoveChild of leaf node")
		return
	}
	delete(n.children, name)
}

// dumpSettings clones the entire tree starting from the root into a map[string]interface{}
//
// If includeDefaults is false, then leafs with default source will be skipped (only useful for the merged tree)
func (n *nodeImpl) dumpSettings(includeDefaults bool) map[string]interface{} {
	res := map[string]interface{}{}

	for _, k := range n.ChildrenKeys() {
		child, _ := n.GetChild(k)
		if child.IsLeafNode() {
			if child.Source() == model.SourceDefault && !includeDefaults {
				continue
			}
			res[k] = child.Get()
		}

		if child.IsInnerNode() {
			childDump := child.dumpSettings(includeDefaults)
			if len(childDump) != 0 {
				res[k] = childDump
			}
		}
	}
	return res
}

// Clone clones a LeafNode
func (n *nodeImpl) Clone() *nodeImpl {
	if n.IsLeafNode() {
		return newLeafNode(n.val, n.source)
	}

	children := make(map[string]*nodeImpl)
	for k, node := range n.children {
		children[k] = node.Clone()
	}
	return newInnerNode(children)
}

// SourceGreaterThan returns true if the source of the current node is greater than the one given as a
// parameter
func (n *nodeImpl) SourceGreaterThan(source model.Source) bool {
	return n.source.IsGreaterThan(source)
}

// Get returns the setting value stored by the leaf
func (n *nodeImpl) Get() interface{} {
	return n.val
}

// IsLeafNode returns true if the node is a leaf
func (n *nodeImpl) IsLeafNode() bool {
	return n.children == nil
}

// IsInnerNode returns true if the node is an inner node
func (n *nodeImpl) IsInnerNode() bool {
	return n.children != nil
}

// ReplaceValue replaces the value in the leaf node
func (n *nodeImpl) ReplaceValue(v interface{}) error {
	if n.IsInnerNode() {
		return errors.New("cannot ReplaceValue of innerNode")
	}
	n.val = v
	return nil
}

// Source returns the source for this leaf
func (n *nodeImpl) Source() model.Source {
	return n.source
}
