// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverdebugimpl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func TestMilestone1ShardIdentityMatchesCurrentServerDebugStats(t *testing.T) {
	debug := fulfillDeps(t, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.SetMetricStatsEnabled(true)
	defer func() {
		d.SetMetricStatsEnabled(false)
		time.Sleep(50 * time.Millisecond)
	}()

	base := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		Mtype:      metrics.GaugeType,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "low"},
		ListenerID: "udp-127.0.0.1:8125",
	}
	reordered := base
	reordered.Tags = []string{"service:web", "env:prod"}
	differentHostOriginAndType := base
	differentHostOriginAndType.Host = "host-b"
	differentHostOriginAndType.Mtype = metrics.CounterType
	differentHostOriginAndType.OriginInfo = taggertypes.OriginInfo{ContainerIDFromSocket: "container-b", Cardinality: "high"}
	differentHostOriginAndType.ListenerID = "uds-unixgram-7"
	differentTags := base
	differentTags.Tags = []string{"env:prod", "service:api"}

	builder := identity.NewBuilder()
	baseShard := builder.Shard(base)
	differentHostShard := builder.Shard(differentHostOriginAndType)
	differentTagsShard := builder.Shard(differentTags)

	d.StoreMetricStats(base)
	d.StoreMetricStats(reordered)
	d.StoreMetricStats(differentHostOriginAndType)
	d.StoreMetricStats(differentTags)

	payload, err := d.GetJSONDebugStats()
	require.NoError(t, err)
	var stats map[ckey.ContextKey]metricStat
	require.NoError(t, json.Unmarshal(payload, &stats))
	require.Len(t, stats, 3)

	baseStat, ok := stats[baseShard.ContextKey]
	require.True(t, ok, "shared shard identity must point at the current serverDebug map entry")
	assert.Equal(t, uint64(2), baseStat.Count, "reordered tags collapse into the same host-aware series")
	assert.Equal(t, baseShard.Client.Name, baseStat.Name)
	assert.ElementsMatch(t, strings.Fields(baseShard.DisplayTags), strings.Fields(baseStat.Tags))

	differentHostStat, ok := stats[differentHostShard.ContextKey]
	require.True(t, ok)
	assert.Equal(t, uint64(1), differentHostStat.Count)
	assert.ElementsMatch(t, strings.Fields(differentHostShard.DisplayTags), strings.Fields(differentHostStat.Tags))

	differentTagsStat, ok := stats[differentTagsShard.ContextKey]
	require.True(t, ok)
	assert.Equal(t, uint64(1), differentTagsStat.Count)
	assert.ElementsMatch(t, strings.Fields(differentTagsShard.DisplayTags), strings.Fields(differentTagsStat.Tags))
}
