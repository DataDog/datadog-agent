// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func TestMilestone1ShardIdentityMatchesCurrentBatcher(t *testing.T) {
	base := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		Mtype:      metrics.GaugeType,
		Value:      1,
		SampleRate: 0.5,
		Timestamp:  123,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "low"},
		ListenerID: "udp-127.0.0.1:8125",
		Source:     metrics.MetricSourceDogstatsd,
	}

	reorderedTags := base
	reorderedTags.Tags = []string{"service:web", "env:prod"}
	changedOrigin := base
	changedOrigin.OriginInfo = taggertypes.OriginInfo{ContainerIDFromSocket: "container-b", Cardinality: "high"}
	changedOrigin.ListenerID = "uds-unixgram-7"
	changedOrigin.Mtype = metrics.CounterType
	changedOrigin.SampleRate = 1
	changedOrigin.Timestamp = 0
	changedHost := base
	changedHost.Host = "host-b"
	changedTags := base
	changedTags.Tags = []string{"env:prod", "service:api"}

	builder := identity.NewBuilder()
	baseShard := builder.Shard(base)

	assert.Equal(t, batchShardContextKey(base), baseShard.ContextKey, "new shard identity helper must match current batcher context key")
	assert.Equal(t, base.Name, baseShard.Client.Name)
	assert.Equal(t, base.Tags, baseShard.Client.Tags)
	assert.Equal(t, base.Host, baseShard.Host)
	assert.Equal(t, baseShard.ContextKey, builder.Shard(reorderedTags).ContextKey, "helper preserves current tag order/dedup semantics")
	assert.Equal(t, baseShard.ContextKey, builder.Shard(changedOrigin).ContextKey, "helper preserves current origin/listener/type ignoring semantics")
	assert.NotEqual(t, baseShard.ContextKey, builder.Shard(changedHost).ContextKey, "helper preserves host sensitivity")
	assert.NotEqual(t, baseShard.ContextKey, builder.Shard(changedTags).ContextKey, "helper preserves tag sensitivity")

	for _, shards := range []int{1, 2, 8, 32} {
		t.Run(fmt.Sprintf("shards_%d", shards), func(t *testing.T) {
			generator := newShardGenerator()
			assert.Equal(t, fastrange(baseShard.ContextKey, shards), identity.ShardIndex(baseShard.ContextKey, shards), "new shard-index helper must match current fastrange formula")
			assert.Equal(t, generator.Generate(base, shards), identity.ShardIndex(baseShard.ContextKey, shards), "new shard identity must select the same current batcher shard")
		})
	}
}

func TestMilestone1ParsedSampleIdentityContracts(t *testing.T) {
	deps := fulfillDepsWithConfigYaml(t, `
dogstatsd_port: __random__
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
`)
	s := deps.Server.(*dsdServer)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	samples, err := s.parseMetricMessage(nil, parser,
		[]byte("test.job.duration.api.sync:42|g|@0.25|#client:tag,host:custom-host,dd.internal.card:high"),
		"container-from-uds", 4242, "udp-127.0.0.1:8125", false, nil)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	sample := samples[0]
	ids := identity.NewBuilder().Resolve(sample)

	assert.Equal(t, "test.job.duration", ids.Client.Name, "identity is based on the parsed/mapped metric name, not the raw wire name")
	assert.ElementsMatch(t, []string{"client:tag", "job_type:api", "job_name:sync"}, ids.Client.Tags, "identity sees parsed client and mapper tags after metadata extraction")
	assert.Equal(t, "custom-host", ids.Shard.Host, "shard identity includes the parsed host")
	assert.Equal(t, batchShardContextKey(sample), ids.Shard.ContextKey)

	assert.Equal(t, sample.Name, ids.BackendSeed.Name)
	assert.Equal(t, sample.Host, ids.BackendSeed.Host)
	assert.Equal(t, sample.Tags, ids.BackendSeed.MetricTags)
	assert.Equal(t, sample.Mtype, ids.BackendSeed.MetricType)
	assert.Equal(t, sample.Source, ids.BackendSeed.Source)
	assert.Equal(t, sample.OriginInfo, ids.BackendSeed.OriginInfo)

	assert.Equal(t, "container-from-uds", ids.Lineage.OriginInfo.ContainerIDFromSocket)
	assert.Equal(t, uint32(4242), ids.Lineage.OriginInfo.LocalData.ProcessID)
	assert.Equal(t, "high", ids.Lineage.OriginInfo.Cardinality)
	assert.Equal(t, "udp-127.0.0.1:8125", ids.Lineage.ListenerID)
}
