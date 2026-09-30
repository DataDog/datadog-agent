// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeStatusGroupWireFormat verifies the runner group on the wire: set
// when the worker belongs to a group, absent otherwise, so an older Cluster
// Agent is unaffected and a payload from an older worker decodes as general.
func TestNodeStatusGroupWireFormat(t *testing.T) {
	data, err := json.Marshal(NodeStatus{LastChange: 42, NodeType: NodeTypeCLCRunner, Group: "kube"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"last_change":42,"node_type":1,"group":"kube"}`, string(data))

	data, err = json.Marshal(NodeStatus{LastChange: 7, NodeType: NodeTypeNodeAgent})
	require.NoError(t, err)
	assert.JSONEq(t, `{"last_change":7,"node_type":2}`, string(data))

	var decoded NodeStatus
	require.NoError(t, json.Unmarshal([]byte(`{"last_change":7,"node_type":2}`), &decoded))
	assert.Empty(t, decoded.Group)
}
