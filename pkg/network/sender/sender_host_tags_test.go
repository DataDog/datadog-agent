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

	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// payloadTagsOnECSFargate builds the payload tags the sender uses on ECS Fargate. The sender itself
// is built first, outside of ECS Fargate: New memoizes process-wide state from the environment
// (e.g. the container host type), which would otherwise leak into the other tests.
func payloadTagsOnECSFargate(t *testing.T, globalTags ...string) *hosttags.PayloadTags {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags(nil, globalTags, nil, nil)
	env.SetFeatures(t, env.ECSFargate)
	return hosttags.NewPayloadTags(hosttags.NewHostTagProvider(), fakeTagger, true)
}

func decodeConnections(t *testing.T, payload []byte) *model.CollectorConnections {
	m, err := model.DecodeMessage(payload)
	require.NoError(t, err)
	c, ok := m.Body.(*model.CollectorConnections)
	require.True(t, ok)
	return c
}

func TestBatchesCarryTheHostTagsInEveryChunk(t *testing.T) {
	const taskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"
	d := mockDirectSender(t, nil)
	d.hostTags = payloadTagsOnECSFargate(t, "cluster_name:cluster", taskARNTag)
	d.maxConnsPerMessage = 1

	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(3)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 3)
	for _, p := range payloads {
		c := decodeConnections(t, p)
		require.Positive(t, c.HostTagsIndex)
		assert.Equal(t, []string{"cluster_name:cluster", taskARNTag}, c.GetTags(int(c.HostTagsIndex)))
	}
}

func TestBatchesWithoutHostTags(t *testing.T) {
	// no expected host tags, which would come from the environment (e.g. DD_TAGS)
	d := mockDirectSender(t, map[string]interface{}{"system_probe_config.expected_tags_duration": 0})
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(1)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 1)
	assert.Negative(t, decodeConnections(t, payloads[0]).HostTagsIndex)
}

// TestNewPayloadTagsOnECSFargate covers how New builds the payload tags: from the sender's tagger,
// with ECS Fargate detected from the environment. It doesn't call New, whose memoized state would
// leak ECS Fargate mode into the other tests.
func TestNewPayloadTagsOnECSFargate(t *testing.T) {
	const taskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags(nil, []string{taskARNTag}, nil, nil)
	syscfg := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{"system_probe_config.expected_tags_duration": 0})
	env.SetFeatures(t, env.ECSFargate)

	assert.Equal(t, []string{taskARNTag}, newPayloadTags(syscfg, fakeTagger).Get())
}
