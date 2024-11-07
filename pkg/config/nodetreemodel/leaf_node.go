// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
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

func newLeafNode(v interface{}, source model.Source) Node {
	return &leafNodeImpl{val: v, source: source}
}

// Clone clones a LeafNode
func (n *leafNodeImpl) Clone() Node {
	return newLeafNode(n.val, n.source)
}

// SourceGreaterOrEqual returns true if the source of the current node is greater or equal to the one given as a
// parameter
func (n *leafNodeImpl) SourceGreaterOrEqual(source model.Source) bool {
	return n.source.IsGreaterOrEqualThan(source)
}

// GetChild returns an error because a leaf has no children
func (n *leafNodeImpl) GetChild(key string) (Node, error) {
	return nil, fmt.Errorf("can't GetChild(%s) of a leaf node", key)
}

// GetAny returns the scalar as an interface
func (n *leafNodeImpl) GetAny() (interface{}, error) {
	return n.val, nil
}

// GetBool returns the scalar as a bool, or an error otherwise
func (n *leafNodeImpl) GetBool() (bool, error) {
	return toBool(n.val)
}

// GetInt returns the scalar as a int, or an error otherwise
func (n *leafNodeImpl) GetInt() (int, error) {
	return toInt(n.val)
}

// GetFloat returns the scalar as a float64, or an error otherwise
func (n *leafNodeImpl) GetFloat() (float64, error) {
	return toFloat(n.val)
}

// GetString returns the scalar as a string, or an error otherwise
func (n *leafNodeImpl) GetString() (string, error) {
	return toString(n.val)
}

// GetTime returns the scalar as a time, or an error otherwise, not implemented
func (n *leafNodeImpl) GetTime() (time.Time, error) {
	return time.Time{}, fmt.Errorf("not implemented")
}

// GetDuration returns the scalar as a duration, or an error otherwise, not implemented
func (n *leafNodeImpl) GetDuration() (time.Duration, error) {
	return time.Duration(0), fmt.Errorf("not implemented")
}

// SetWithSource assigns a value in the config, for the given source
func (n *leafNodeImpl) SetWithSource(newValue interface{}, source model.Source) error {
	// TODO: enforce type-checking, return an error if type changes
	n.val = newValue
	n.source = source
	// TODO: Record previous value and source
	return nil
}

// Source returns the source for this leaf
func (n *leafNodeImpl) Source() model.Source {
	return n.source
}

// Set is not implemented for a leaf node
func (n *leafNodeImpl) Set([]string, interface{}, model.Source) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
