// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func TestHasCompleteConnectionGroup(t *testing.T) {
	payload := func(groupID, groupSize int32, connections int) *aggregator.Connections {
		return &aggregator.Connections{CollectorConnections: agentmodel.CollectorConnections{
			GroupId:     groupID,
			GroupSize:   groupSize,
			Connections: make([]*agentmodel.Connection, connections),
		}}
	}
	tests := []struct {
		name     string
		payloads []*aggregator.Connections
		want     bool
	}{
		{name: "no payloads"},
		{
			name:     "single chunk is not a split group",
			payloads: []*aggregator.Connections{payload(1, 1, 600)},
		},
		{
			name:     "separate check runs cannot be combined",
			payloads: []*aggregator.Connections{payload(1, 1, 300), payload(2, 1, 300)},
		},
		{
			name:     "incomplete group despite enough connections",
			payloads: []*aggregator.Connections{payload(1, 3, 600), payload(1, 3, 600)},
		},
		{
			name:     "complete group below connection threshold",
			payloads: []*aggregator.Connections{payload(1, 2, 300), payload(1, 2, 299)},
		},
		{
			name:     "complete group at connection threshold",
			payloads: []*aggregator.Connections{payload(1, 2, 400), payload(1, 2, 200)},
			want:     true,
		},
		{
			name:     "complete three chunk burst",
			payloads: []*aggregator.Connections{payload(1, 3, 600), payload(1, 3, 600), payload(1, 3, 327)},
			want:     true,
		},
		{
			name: "burst remains valid after normal check runs",
			payloads: []*aggregator.Connections{
				payload(1, 3, 600), payload(1, 3, 600), payload(1, 3, 327),
				payload(2, 1, 19), payload(3, 1, 27), payload(4, 1, 32),
			},
			want: true,
		},
		{
			name: "delayed chunks can arrive out of order across check runs",
			payloads: []*aggregator.Connections{
				payload(1, 3, 327), payload(2, 1, 19), payload(1, 3, 600),
				payload(3, 1, 27), payload(1, 3, 600),
			},
			want: true,
		},
		{
			name:     "inconsistent group sizes",
			payloads: []*aggregator.Connections{payload(1, 2, 600), payload(1, 3, 600)},
		},
		{
			name:     "more chunks than advertised",
			payloads: []*aggregator.Connections{payload(1, 2, 600), payload(1, 2, 600), payload(1, 2, 327)},
		},
		{
			name: "incomplete group does not hide a complete group",
			payloads: []*aggregator.Connections{
				payload(1, 3, 600), payload(2, 2, 600), payload(2, 2, 327),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasCompleteConnectionGroup(tt.payloads))
		})
	}
}
