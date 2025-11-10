// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = fmt.Errorf("not found")

func mapToMapString(m reflect.Value) map[string]interface{} {
	if v, ok := m.Interface().(map[string]interface{}); ok {
		// no need to convert the map
		return v
	}

	res := make(map[string]interface{}, m.Len())

	iter := m.MapRange()
	for iter.Next() {
		k := iter.Key()
		mk := ""
		if k.Kind() == reflect.String {
			mk = k.Interface().(string)
		} else {
			mk = fmt.Sprintf("%s", k.Interface())
		}
		res[mk] = iter.Value().Interface()
	}
	return res
}

// valid kinds to call IsNil on
var nillableKinds = []reflect.Kind{reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice}

// IsNilValue returns true if a is nil, or a is an interface with nil data
func IsNilValue(a interface{}) bool {
	if a == nil {
		return true
	}
	rv := reflect.ValueOf(a)
	// check if IsNil may be called in order to avoid a panic
	if slices.Contains(nillableKinds, rv.Kind()) {
		return reflect.ValueOf(a).IsNil()
	}
	return false
}

// NewNodeTree will recursively create nodes from the input value to construct a tree
func NewNodeTree(v interface{}, source model.Source) (Node, error) {
	if IsNilValue(v) {
		// nil as a value acts as the zero value, and the cast library will correctly
		// convert it to zero values for the types we handle
		return newLeafNode(nil, source), nil
	}
	switch it := v.(type) {
	case []interface{}:
		return newLeafNode(it, source), nil
	}

	// handle all map types that can be converted to map[string]interface{}
	if v := reflect.ValueOf(v); v.Kind() == reflect.Map {
		children, err := makeChildNodeTrees(mapToMapString(v), source)
		if err != nil {
			return nil, err
		}
		return newInnerNode(children), nil
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
	Merge(InnerNode) (InnerNode, error)
	SetAt([]string, interface{}, model.Source) error
	InsertChildNode(string, Node)
	RemoveChild(string)
	DumpSettings(func(model.Source) bool) map[string]interface{}
}

// LeafNode represents a leaf node of the config
type LeafNode interface {
	Node
	Get() interface{}
	ReplaceValue(v interface{}) error
	Source() model.Source
	SourceGreaterThan(model.Source) bool
}
