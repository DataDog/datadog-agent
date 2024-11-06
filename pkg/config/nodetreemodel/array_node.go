// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ArrayNode represents a node with ordered, numerically indexed set of children
type ArrayNode interface {
	Size() int
	Index(int) (Node, error)
}

type arrayNodeImpl struct {
	nodes  []Node
	source model.Source
}

var _ ArrayNode = (*arrayNodeImpl)(nil)
var _ Node = (*arrayNodeImpl)(nil)

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

// Clone clones a LeafNode
func (n *arrayNodeImpl) Clone() Node {
	clone := &arrayNodeImpl{nodes: make([]Node, len(n.nodes)), source: n.source}
	for idx, n := range n.nodes {
		clone.nodes[idx] = n.Clone()
	}
	return clone
}

// SourceGreaterOrEqual returns true if the source of the current node is greater or equal to the one given as a
// parameter
func (n *arrayNodeImpl) SourceGreaterOrEqual(source model.Source) bool {
	return n.source.IsGreaterOrEqualThan(source)
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

// Source returns the source for this leaf
func (n *arrayNodeImpl) Source() model.Source {
	return n.source
}

// Set is not implemented for an array node
func (n *arrayNodeImpl) Set([]string, interface{}, model.Source) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
