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

func TestBatchesIncludeTaskARNOnECSFargate(t *testing.T) {
	const taskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"
	env.SetFeatures(t, env.ECSFargate)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags([]string{"cluster_name:cluster"}, []string{taskARNTag}, nil, nil)

	d := mockDirectSenderWithTagger(t, nil, fakeTagger)
	d.maxConnsPerMessage = 1
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(3)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 3)
	for _, p := range payloads {
		m, err := model.DecodeMessage(p)
		require.NoError(t, err)
		c, ok := m.Body.(*model.CollectorConnections)
		require.True(t, ok)
		require.Positive(t, c.HostTagsIndex)
		assert.ElementsMatch(t, []string{"cluster_name:cluster", taskARNTag}, c.GetTags(int(c.HostTagsIndex)))
	}
}

func TestBatchesHaveNoGlobalTagsOutsideECSFargate(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags([]string{"cluster_name:cluster"}, []string{"task_arn:arn"}, nil, nil)

	d := mockDirectSenderWithTagger(t, nil, fakeTagger)
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: makeConnections(1)}}
	payloads := slices.Collect(d.batches(conns, 1))

	require.Len(t, payloads, 1)
	m, err := model.DecodeMessage(payloads[0])
	require.NoError(t, err)
	c := m.Body.(*model.CollectorConnections)
	if c.HostTagsIndex > 0 {
		assert.NotContains(t, c.GetTags(int(c.HostTagsIndex)), "task_arn:arn")
	}
}
