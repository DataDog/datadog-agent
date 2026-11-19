// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || (windows && npm))

package sender

import (
	"slices"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/network"
)

const testTaskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

// noExpectedHostTags disables the expected host tags, which come from the environment (e.g. DD_TAGS).
var noExpectedHostTags = map[string]interface{}{"system_probe_config.expected_tags_duration": 0}

func decodeConnections(t *testing.T, payload []byte) *model.CollectorConnections {
	m, err := model.DecodeMessage(payload)
	require.NoError(t, err)
	c, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)
	return c
}

func TestBatchesCarryTheECSTaskTagsInEveryChunk(t *testing.T) {
	d := mockDirectSender(t, noExpectedHostTags)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags(nil, []string{"cluster_name:cluster", testTaskARNTag}, nil, nil)
	d.tagger = fakeTagger
	d.maxConnsPerMessage = 1
	// set after New: it memoizes state from the environment (e.g. the container host type), which
	// would otherwise leak ECS Fargate mode into the other tests
	env.SetFeatures(t, env.ECSFargate)

	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(3)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 3)
	for _, p := range payloads {
		c := decodeConnections(t, p)
		require.Positive(t, c.HostTagsIndex)
		assert.Equal(t, []string{"cluster_name:cluster", testTaskARNTag}, c.GetTags(int(c.HostTagsIndex)))
	}
}

func TestBatchesHaveNoECSTaskTagsElsewhere(t *testing.T) {
	d := mockDirectSender(t, noExpectedHostTags)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags(nil, []string{testTaskARNTag}, nil, nil)
	d.tagger = fakeTagger

	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(1)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 1)
	assert.Negative(t, decodeConnections(t, payloads[0]).HostTagsIndex)
}
