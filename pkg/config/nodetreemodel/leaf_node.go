// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type leafNodeImpl struct {
	// val must be a scalar kind
	val    interface{}
	source model.Source
}

var _ LeafNode = (*leafNodeImpl)(nil)
var _ Node = (*leafNodeImpl)(nil)

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

func newLeafNode(v interface{}, source model.Source) Node {
	return &leafNodeImpl{val: v, source: source}
}

// Clone clones a LeafNode
func (n *leafNodeImpl) Clone() Node {
	return newLeafNode(n.val, n.source)
}

// SourceGreaterThan returns true if the source of the current node is greater than the one given as a
// parameter
func (n *leafNodeImpl) SourceGreaterThan(source model.Source) bool {
	return n.source.IsGreaterThan(source)
}

// GetChild returns an error because a leaf has no children
func (n *leafNodeImpl) GetChild(key string) (Node, error) {
	return nil, fmt.Errorf("can't GetChild(%s) of a leaf node", key)
}

// Get returns the setting value stored by the leaf
func (n *leafNodeImpl) Get() interface{} {
	return n.val
}

// ReplaceValue replaces the value in the leaf node
func (n *leafNodeImpl) ReplaceValue(v interface{}) error {
	n.val = v
	return nil
}

// Source returns the source for this leaf
func (n *leafNodeImpl) Source() model.Source {
	return n.source
}
