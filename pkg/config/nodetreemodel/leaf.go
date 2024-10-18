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

// ArrayNode represents a node with ordered, numerically indexed set of children
type ArrayNode interface {
	Size() int
	Index(int) (Node, error)
}

type arrayNodeImpl struct {
	nodes []Node
}

func newArrayNodeImpl(v []interface{}, source model.Source) (Node, error) {
	nodes := make([]Node, 0, len(v))
	for _, it := range v {
		if n, ok := it.(Node); ok {
			nodes = append(nodes, n)
			continue
		}
		n, err := NewNode(it, source)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return &arrayNodeImpl{nodes: nodes}, nil
}

// GetChild returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) GetChild(string) (Node, error) {
	return nil, fmt.Errorf("arrayNodeImpl.GetChild not implemented")
}

// ChildrenKeys returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("arrayNodeImpl.ChildrenKeys not implemented")
}

// Size returns number of children in the list
func (n *arrayNodeImpl) Size() int {
	return len(n.nodes)
}

// Index returns the kth element of the list
func (n *arrayNodeImpl) Index(k int) (Node, error) {
	if k < 0 || k >= len(n.nodes) {
		return nil, ErrNotFound
	}
	return n.nodes[k], nil
}

var _ ArrayNode = (*arrayNodeImpl)(nil)
var _ Node = (*arrayNodeImpl)(nil)

// leafNode represents a leaf with a scalar value

type leafNodeImpl struct {
	// val must be a scalar kind
	val    interface{}
	source model.Source
}

func newLeafNodeImpl(v interface{}, source model.Source) (Node, error) {
	if isScalar(v) {
		return &leafNodeImpl{val: v, source: source}, nil
	}
	return nil, fmt.Errorf("cannot create leaf node from %v of type %T", v, v)
}

var _ LeafNode = (*leafNodeImpl)(nil)
var _ Node = (*leafNodeImpl)(nil)

// GetChild returns an error because a leaf has no children
func (n *leafNodeImpl) GetChild(key string) (Node, error) {
	return nil, fmt.Errorf("can't GetChild(%s) of a leaf node", key)
}

// ChildrenKeys returns an error because a leaf has no children
func (n *leafNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("can't get ChildrenKeys of a leaf node")
}

// GetAny returns the scalar as an interface
func (n *leafNodeImpl) GetAny() (interface{}, error) {
	return n, nil
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

// Set assigns a value in the config, for the given source
func (n *leafNodeImpl) SetWithSource(newValue interface{}, source model.Source) error {
	// TODO: enforce type-checking, return an error if type changes
	n.val = newValue
	n.source = source
	// TODO: Record previous value and source
	return nil
}
