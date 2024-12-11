// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"unsafe"
)

// Node describes an edge of a dot node
type Node struct {
	ID        GraphID
	Label     string
	Size      int
	Color     string
	FillColor string
	Shape     string
	IsTable   bool
}

// Edge describes an edge of a dot edge
type Edge struct {
	From  GraphID
	To    GraphID
	Color string
}

// Graph describes a dot graph
type Graph struct {
	Title string
	Nodes map[GraphID]*Node
	Edges []*Edge
}

// EncodeDOT encodes an activity dump in the DOT format
func (g *Graph) EncodeDOT(tmpl string) (*bytes.Buffer, error) {
	t := template.Must(template.New("tmpl").Parse(tmpl))
	raw := new(bytes.Buffer)
	if err := t.Execute(raw, g); err != nil {
		return nil, err
	}
	return raw, nil
}

// GraphID represents an ID used in a graph, combination of NodeIDs
type GraphID struct {
	raw string
}

// NewGraphID returns a new GraphID based on the provided NodeIDs
func NewGraphID(id NodeID) GraphID {
	return NewGraphIDWithDescription("", id)
}

// NewGraphIDWithDescription returns a new GraphID based on a description and on the provided NodeIDs
func NewGraphIDWithDescription(description string, id NodeID) GraphID {
	if description == "" {
		description = "node"
	}
	return GraphID{
		raw: fmt.Sprintf("%s_%d", description, id.inner),
	}
}

// Derive a GraphID from a set of nodes
func (id *GraphID) Derive(ids ...NodeID) GraphID {
	var builder strings.Builder
	builder.WriteString(id.raw)
	for _, sub := range ids {
		builder.WriteString(fmt.Sprintf("_%d", sub.inner))
	}
	return GraphID{
		raw: builder.String(),
	}
}

func (id GraphID) String() string {
	return id.raw
}

// NodeID represents the ID of a Node
type NodeID struct {
	inner uint64
}

// NewNodeID returns a new node ID with the specified value
func NewNodeID(inner uint64) NodeID {
	return NodeID{
		inner: inner,
	}
}

// NewRandomNodeID returns a new random NodeID
func NewRandomNodeID() NodeID {
	return NodeID{
		inner: RandNonZeroUint64(),
	}
}

// NewNodeIDFromPtr returns a new NodeID based on a pointer value
func NewNodeIDFromPtr[T any](v *T) NodeID {
	ptr := uintptr(unsafe.Pointer(v))
	return NodeID{
		inner: uint64(ptr),
	}
}

// IsUnset checks if the NodeID is unset
func (id NodeID) IsUnset() bool {
	return id.inner == 0
}
