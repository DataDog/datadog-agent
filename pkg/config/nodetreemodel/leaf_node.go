// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

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

func isSlice(v interface{}) bool {
	rval := reflect.ValueOf(v)
	return rval.Kind() == reflect.Slice
}

func newLeafNode(v interface{}, source model.Source) *nodeImpl {
	return &nodeImpl{val: v, source: source}
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
