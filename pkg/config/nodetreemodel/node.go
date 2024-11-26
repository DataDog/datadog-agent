// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = fmt.Errorf("not found")

func mapInterfaceToMapString(m map[interface{}]interface{}) map[string]interface{} {
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		mk := ""
		if str, ok := k.(string); ok {
			mk = str
		} else {
			mk = fmt.Sprintf("%s", k)
		}
		res[mk] = v
	}
	return res
}

// NewNodeTree will recursively create nodes from the input value to construct a tree
func NewNodeTree(v interface{}, source model.Source) (Node, error) {
	switch it := v.(type) {
	case map[interface{}]interface{}:
		children, err := makeChildNodeTrees(mapInterfaceToMapString(it), source)
		if err != nil {
			return nil, err
		}
		return newInnerNode(children), nil
	case map[string]interface{}:
		children, err := makeChildNodeTrees(it, source)
		if err != nil {
			return nil, err
		}
		return newInnerNode(children), nil
	case []interface{}:
		return newLeafNode(it, source), nil
	}
	if isScalar(v) {
		return newLeafNode(v, source), nil
	}
	// Finally, try determining node type using reflection, should only be needed for unit tests that
	// supply data that isn't one of the "plain" types produced by parsing json, yaml, etc
	node, err := asReflectionNode(v)
	if err == errUnknownConversion {
		return nil, fmt.Errorf("could not create node from: %v of type %T", v, v)
	}
	return node, err
}

func makeChildNodeTrees(input map[string]interface{}, source model.Source) (map[string]Node, error) {
	children := make(map[string]Node)
	for k, v := range input {
		node, err := NewNodeTree(v, source)
		if err != nil {
			return nil, err
		}
		children[k] = node
	}
	return children, nil
}

// NodeType represents node types in the tree (ie: inner or leaf)
type NodeType int

const (
	// InnerType is a inner node in the config
	InnerType NodeType = iota
	// LeafType is a leaf node in the config
	LeafType
	// MissingType is a none-object representing when a child node is missing
	MissingType
)

// Node represents a arbitrary node
type Node interface {
	Clone() Node
	GetChild(string) (Node, error)
}

// InnerNode represents an inner node in the config
type InnerNode interface {
	Node
	HasChild(string) bool
	ChildrenKeys() []string
	Merge(InnerNode) error
	SetAt([]string, interface{}, model.Source) (bool, error)
	InsertChildNode(string, Node)
	makeRemapCase()
	DumpSettings(func(model.Source) bool) map[string]interface{}
}

// LeafNode represents a leaf node of the config
type LeafNode interface {
	Node
	Get() interface{}
	Source() model.Source
	SourceGreaterOrEqual(model.Source) bool
}
