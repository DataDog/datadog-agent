// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNodeID(t *testing.T) {
	tests := []struct {
		name  string
		value uint64
	}{
		{"zero", 0},
		{"one", 1},
		{"max", ^uint64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := NewNodeID(tt.value)
			assert.Equal(t, tt.value == 0, id.IsUnset())
		})
	}
}

func TestNewRandomNodeID(t *testing.T) {
	id := NewRandomNodeID()
	assert.False(t, id.IsUnset(), "Random NodeID should never be unset")
}

func TestNewRandomNodeID_Uniqueness(t *testing.T) {
	ids := make(map[uint64]bool)
	for i := 0; i < 100; i++ {
		id := NewRandomNodeID()
		ids[id.inner] = true
	}
	assert.Greater(t, len(ids), 95, "Expected most random NodeIDs to be unique")
}

func TestNewNodeIDFromPtr(t *testing.T) {
	var x int = 42
	id := NewNodeIDFromPtr(&x)
	assert.False(t, id.IsUnset())
}

func TestNodeID_IsUnset(t *testing.T) {
	unset := NodeID{inner: 0}
	set := NodeID{inner: 123}

	assert.True(t, unset.IsUnset())
	assert.False(t, set.IsUnset())
}

func TestNewGraphID(t *testing.T) {
	nodeID := NewNodeID(42)
	graphID := NewGraphID(nodeID)

	assert.Equal(t, "node_42", graphID.String())
}

func TestNewGraphIDWithDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		nodeID      uint64
		expected    string
	}{
		{
			name:        "with description",
			description: "process",
			nodeID:      123,
			expected:    "process_123",
		},
		{
			name:        "empty description uses default",
			description: "",
			nodeID:      456,
			expected:    "node_456",
		},
		{
			name:        "custom description",
			description: "file",
			nodeID:      789,
			expected:    "file_789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID := NewNodeID(tt.nodeID)
			graphID := NewGraphIDWithDescription(tt.description, nodeID)
			assert.Equal(t, tt.expected, graphID.String())
		})
	}
}

func TestGraphID_Derive(t *testing.T) {
	nodeID := NewNodeID(1)
	graphID := NewGraphID(nodeID)

	derived := graphID.Derive(NewNodeID(2), NewNodeID(3))
	assert.Equal(t, "node_1_2_3", derived.String())

	// Derive from derived
	further := derived.Derive(NewNodeID(4))
	assert.Equal(t, "node_1_2_3_4", further.String())
}

func TestGraphID_String(t *testing.T) {
	graphID := GraphID{raw: "test_123"}
	assert.Equal(t, "test_123", graphID.String())
}

func TestGraph_EncodeDOT(t *testing.T) {
	graph := &Graph{
		Title: "Test Graph",
		Nodes: map[GraphID]*Node{
			NewGraphID(NewNodeID(1)): {
				ID:    NewGraphID(NewNodeID(1)),
				Label: "Node 1",
			},
		},
		Edges: []*Edge{},
	}

	tmpl := `digraph {
  label="{{.Title}}"
  {{range $id, $node := .Nodes}}
  {{$id}} [label="{{$node.Label}}"]
  {{end}}
}`

	result, err := graph.EncodeDOT(tmpl)
	require.NoError(t, err)
	require.NotNil(t, result)

	output := result.String()
	assert.Contains(t, output, "Test Graph")
	assert.Contains(t, output, "Node 1")
}

func TestGraph_EncodeDOT_ExecutionError(t *testing.T) {
	graph := &Graph{
		Title: "Test",
	}

	// Template that references non-existent field causes execution error
	tmpl := `{{.NonExistentField.SubField}}`
	_, err := graph.EncodeDOT(tmpl)
	assert.Error(t, err)
}

func TestGraph_EncodeDOT_WithSubGraphs(t *testing.T) {
	subGraph := &SubGraph{
		Name:  "cluster_0",
		Title: "SubGraph 1",
		Nodes: map[GraphID]*Node{
			NewGraphID(NewNodeID(10)): {
				ID:    NewGraphID(NewNodeID(10)),
				Label: "SubNode",
			},
		},
	}

	graph := &Graph{
		Title:     "Main Graph",
		SubGraphs: []*SubGraph{subGraph},
	}

	tmpl := `digraph {
  label="{{.Title}}"
  {{range .SubGraphs}}
  subgraph {{.Name}} {
    label="{{.Title}}"
  }
  {{end}}
}`

	result, err := graph.EncodeDOT(tmpl)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "SubGraph 1")
}

func TestNode_Fields(t *testing.T) {
	node := &Node{
		ID:        NewGraphID(NewNodeID(1)),
		Label:     "Test Node",
		Size:      10,
		Color:     "blue",
		FillColor: "lightblue",
		Shape:     "box",
		IsTable:   true,
	}

	assert.Equal(t, "Test Node", node.Label)
	assert.Equal(t, 10, node.Size)
	assert.Equal(t, "blue", node.Color)
	assert.Equal(t, "lightblue", node.FillColor)
	assert.Equal(t, "box", node.Shape)
	assert.True(t, node.IsTable)
}

func TestEdge_Fields(t *testing.T) {
	edge := &Edge{
		From:         NewGraphID(NewNodeID(1)),
		To:           NewGraphID(NewNodeID(2)),
		Color:        "red",
		HasArrowHead: true,
		Label:        "connects",
		IsTable:      false,
	}

	assert.Equal(t, "node_1", edge.From.String())
	assert.Equal(t, "node_2", edge.To.String())
	assert.Equal(t, "red", edge.Color)
	assert.True(t, edge.HasArrowHead)
	assert.Equal(t, "connects", edge.Label)
	assert.False(t, edge.IsTable)
}

func TestSubGraph_Fields(t *testing.T) {
	sg := &SubGraph{
		Name:      "cluster_test",
		Title:     "Test SubGraph",
		TitleSize: 12,
		Color:     "green",
		Nodes:     make(map[GraphID]*Node),
		Edges:     []*Edge{},
	}

	assert.Equal(t, "cluster_test", sg.Name)
	assert.Equal(t, "Test SubGraph", sg.Title)
	assert.Equal(t, 12, sg.TitleSize)
	assert.Equal(t, "green", sg.Color)
}
