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

func newInnerNodeImpl(v map[string]interface{}, source model.Source) (*innerNode, error) {
	children := map[string]Node{}
	for name, value := range v {
		n, err := NewNode(value, source)
		if err != nil {
			return nil, err
		}
		children[name] = n
	}
	return &innerNode{val: children, remapCase: makeRemapCase(children)}, nil
}

var _ Node = (*innerNode)(nil)

///////

// creates a map that converts keys from their lower-cased version to their original case
func makeRemapCase(m map[string]Node) map[string]string {
	remap := make(map[string]string)
	for k := range m {
		remap[strings.ToLower(k)] = k
	}
	return remap
}

/////

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *innerNode) GetChild(key string) (Node, error) {
	mkey := n.remapCase[strings.ToLower(key)]
	child, found := n.val[mkey]
	if !found {
		return nil, ErrNotFound
	}
	return child, nil
}

// Merge mergs src node within current tree
func (n *innerNode) Merge(srcNode Node) error {
	src, ok := srcNode.(*innerNode)
	if !ok {
		return fmt.Errorf("can't merge leaf into a node")
	}

	childrenNames := maps.Keys(src.val)
	// map keys are iterated non-deterministically, sort them
	slices.Sort(childrenNames)

	for _, name := range childrenNames {
		child := src.val[name]
		srcLeaf, srcIsLeaf := child.(*leafNodeImpl)

		if _, ok := n.val[name]; !ok {
			// child from src is unknown, we must create a new node
			if srcIsLeaf {
				n.val[name] = &leafNodeImpl{
					val:    srcLeaf.val,
					source: srcLeaf.source,
				}
			} else {
				newNode := &innerNode{
					val:       map[string]Node{},
					remapCase: map[string]string{},
				}
				if err := newNode.Merge(src.val[name]); err != nil {
					return err
				}
				n.val[name] = newNode
			}
		} else {
			// We alredy have child with the same name: update our child
			dstLeaf, dstIsLeaf := n.val[name].(*leafNodeImpl)
			if srcIsLeaf != dstIsLeaf {
				return fmt.Errorf("tree conflict, can't merge lead and non leaf nodes for '%s'", name)
			}

			if srcIsLeaf {
				if srcLeaf.source.IsGreaterOrEqualThan(dstLeaf.source) {
					dstLeaf.val = srcLeaf.val
					dstLeaf.source = srcLeaf.source
				}
			} else {
				if err := n.val[name].(*innerNode).Merge(child); err != nil {
					return err
				}
			}
		}
	}
	n.remapCase = makeRemapCase(n.val)
	return nil
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *innerNode) ChildrenKeys() ([]string, error) {
	mapkeys := maps.Keys(n.val)
	// map keys are iterated non-deterministically, sort them
	slices.Sort(mapkeys)
	return mapkeys, nil
}

// Set sets a value in the tree by either creating a leaf node or updating one if the priority is equal or higher than
// the existing one. The function returns true if an update was done or false if nothing was changed.
//
// The key parts should already be lowercased.
func (n *innerNode) Set(key []string, value interface{}, source model.Source) (bool, error) {
	keyLen := len(key)
	if keyLen == 0 {
		return false, fmt.Errorf("empty key given to Set")
	}

	defer func() { n.remapCase = makeRemapCase(n.val) }()

	part := key[0]

	if keyLen == 1 {
		if leaf, ok := n.val[part].(LeafNode); ok {
			if leaf.Source().IsGreaterOrEqualThan(source) {
				n.val[part], _ = newLeafNodeImpl(value, source) // TODO: handle non scalar value or merge all leaf type into one
				return true, nil
			}
		} else {
			return false, fmt.Errorf("can't overrides inner node with a leaf node")
		}
	} else {
		// new node case
		if _, ok := n.val[part]; !ok {
			node, _ := newInnerNodeImpl(nil, source)
			n.val[part] = node
			return node.Set(key[1:keyLen-1], value, source)
		}

		// update node case
		node := n.val[part]
		if _, ok := node.(*innerNode); !ok {
			return false, fmt.Errorf("can't update a leaf node into a inner node")
		}
		return node.Set(key[1:keyLen-1], value, source)
	}
	return false, nil
}
