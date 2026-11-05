// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import "testing"

// backendOnlyNodeTypes are resource types the backend uses for resources it
// synthesizes itself (K8S_GENERICWORKLOAD, K8S_INFERENCEWORKLOAD). The backend
// casts agent node types directly to its own resource enum, so the agent must
// never send these values.
var backendOnlyNodeTypes = map[NodeType]string{
	31: "K8S_GENERICWORKLOAD",
	32: "K8S_INFERENCEWORKLOAD",
}

func TestNodeTypesSkipBackendOnlyValues(t *testing.T) {
	for _, nodeType := range NodeTypes() {
		if backendType, ok := backendOnlyNodeTypes[nodeType]; ok {
			t.Errorf("node type %s uses value %d, which the backend reserves for %s", nodeType, nodeType, backendType)
		}
	}
}
