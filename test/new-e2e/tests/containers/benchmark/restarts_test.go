// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareRestarts(t *testing.T) {
	// A node Agent which restarted once while its node was starting, before the first record
	before := map[string]int32{"agent-a/agent": 1, "agent-a/trace-agent": 0, "cluster-agent-a/cluster-agent": 0}

	tests := []struct {
		name  string
		after map[string]int32
		err   string
	}{
		{
			name:  "no restart",
			after: map[string]int32{"agent-a/agent": 1, "agent-a/trace-agent": 0, "cluster-agent-a/cluster-agent": 0},
		},
		{
			name:  "restarts",
			after: map[string]int32{"agent-a/agent": 3, "agent-a/trace-agent": 0, "cluster-agent-a/cluster-agent": 1},
			err:   "agent-a/agent 1→3, cluster-agent-a/cluster-agent 0→1",
		},
		{
			name:  "pod replaced",
			after: map[string]int32{"agent-a/agent": 1, "agent-a/trace-agent": 0, "cluster-agent-b/cluster-agent": 0},
			err:   "cluster-agent-a/cluster-agent: pod replaced, cluster-agent-b/cluster-agent: new pod",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompareRestarts(before, tt.after)
			if tt.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.err)
			}
		})
	}
}
