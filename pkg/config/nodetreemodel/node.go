// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"golang.org/x/exp/maps"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = fmt.Errorf("not found")

// NewNode constructs a Node from either a map, a slice, or a scalar value
func NewNode(v interface{}, source model.Source) (Node, error) {
	switch it := v.(type) {
	case map[interface{}]interface{}:
		return newMapNodeImpl(mapInterfaceToMapString(it), source)
	case map[string]interface{}:
		return newMapNodeImpl(it, source)
	case []interface{}:
		return newArrayNodeImpl(it, source)
	}
	if isScalar(v) {
		return newLeafNodeImpl(v, source)
	}
	// Finally, try determining node type using reflection, should only be needed for unit tests that
	// supply data that isn't one of the "plain" types produced by parsing json, yaml, etc
	node, err := asReflectionNode(v)
	if err == errUnknownConversion {
		return nil, fmt.Errorf("could not create node from: %v of type %T", v, v)
	}
	return node, err
}

// Node represents an arbitrary node
type Node interface {
	GetChild(string) (Node, error)
	ChildrenKeys() ([]string, error)
}

// LeafNode represents a leaf node of the config
type LeafNode interface {
	GetAny() (interface{}, error)
	GetBool() (bool, error)
	GetInt() (int, error)
	GetFloat() (float64, error)
	GetString() (string, error)
	GetTime() (time.Time, error)
	GetDuration() (time.Duration, error)
	SetWithSource(interface{}, model.Source) error
}

// innerNode represents an non-leaf node of the config
type innerNode struct {
	val map[string]Node
	// remapCase maps each lower-case key to the original case. This
	// enables GetChild to retrieve values using case-insensitive keys
	remapCase map[string]string
}

func newMapNodeImpl(v map[string]interface{}, source model.Source) (*innerNode, error) {
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

func isScalar(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case bool, string, float32, float64, time.Time, time.Duration:
		return true
	default:
		return false
	}
}

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
				if srcLeaf.source.IsGreaterThan(dstLeaf.source) {
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
