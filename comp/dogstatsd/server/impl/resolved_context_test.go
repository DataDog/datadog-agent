// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func TestMilestone2BatcherUsesPrecomputedShardIdentity(t *testing.T) {
	const shards = 8
	sample := metrics.MetricSample{
		Name:  "identity.metric",
		Host:  "host-a",
		Tags:  []string{"env:prod", "service:web", "env:prod"},
		Mtype: metrics.GaugeType,
	}
	context := identity.NewBuilder().ResolveHotPath(sample)

	b := &batcher{
		samples:       make([]metrics.MetricSampleBatch, shards),
		samplesCount:  make([]int, shards),
		pipelineCount: shards,
	}
	for shard := range b.samples {
		b.samples[shard] = make(metrics.MetricSampleBatch, 4)
	}

	b.appendSampleWithContext(sample, context)

	expectedShard := identity.ShardIndex(context.Shard.ContextKey, shards)
	for shard, count := range b.samplesCount {
		if uint32(shard) == expectedShard {
			assert.Equal(t, 1, count)
			assert.Equal(t, sample, b.samples[shard][0])
		} else {
			assert.Zero(t, count)
		}
	}
}

func TestMilestone2ParsePacketsCarriesResolvedSampleContext(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                        listeners.RandomPortName,
		"histogram_copy_to_distribution":        true,
		"histogram_copy_to_distribution_prefix": "dist.",
	})
	s := deps.Server.(*dsdServer)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	batcher := &resolvedContextBatcher{needContext: true}

	s.parsePackets(batcher, parser, identity.NewBuilder(), genTestPackets([]byte("identity.metric:1|g|#env:prod,service:web,host:custom-host\nhist.metric:2|h|#env:prod,bucket:p50")), metrics.MetricSampleBatch{}, nil)

	require.Len(t, batcher.samples, 3)
	require.Len(t, batcher.contexts, 3)

	for idx, sample := range batcher.samples {
		assert.Equal(t, batchShardContextKey(sample), batcher.contexts[idx].Shard.ContextKey, "precomputed shard identity must match the sample handed to the batcher")
		assert.Equal(t, sample.Name, batcher.contexts[idx].Client.Name)
		assert.Equal(t, sample.Tags, batcher.contexts[idx].Client.Tags)
	}

	assert.Equal(t, "identity.metric", batcher.samples[0].Name)
	assert.Equal(t, "custom-host", batcher.contexts[0].Shard.Host)
	assert.Equal(t, "hist.metric", batcher.samples[1].Name)
	assert.Equal(t, metrics.HistogramType, batcher.samples[1].Mtype)
	assert.Equal(t, "dist.hist.metric", batcher.samples[2].Name, "histogram copy gets its own context after the name/type rewrite")
	assert.Equal(t, metrics.DistributionType, batcher.samples[2].Mtype)
	assert.Equal(t, "dist.hist.metric", batcher.contexts[2].Shard.Client.Name)
}

type resolvedContextBatcher struct {
	needContext  bool
	contexts     []identity.HotPathContext
	lateContexts []identity.HotPathContext
	samples      []metrics.MetricSample
	lateSamples  []metrics.MetricSample
	events       []*event.Event
	checks       []*servicecheck.ServiceCheck
}

func (b *resolvedContextBatcher) appendSample(sample metrics.MetricSample) {
	b.samples = append(b.samples, sample)
}

func (b *resolvedContextBatcher) appendSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	b.samples = append(b.samples, sample)
	b.contexts = append(b.contexts, context)
}

func (b *resolvedContextBatcher) appendLateSample(sample metrics.MetricSample) {
	b.lateSamples = append(b.lateSamples, sample)
}

func (b *resolvedContextBatcher) appendLateSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	b.lateSamples = append(b.lateSamples, sample)
	b.lateContexts = append(b.lateContexts, context)
}

func (b *resolvedContextBatcher) appendColumnarV3SampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	b.appendSampleWithContext(sample, context)
}

func (b *resolvedContextBatcher) appendEvent(event *event.Event) {
	b.events = append(b.events, event)
}

func (b *resolvedContextBatcher) appendServiceCheck(serviceCheck *servicecheck.ServiceCheck) {
	b.checks = append(b.checks, serviceCheck)
}

func (b *resolvedContextBatcher) needsSampleContext() bool { return b.needContext }
func (b *resolvedContextBatcher) flush()                   {}
