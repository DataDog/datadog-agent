// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestSegmentShadowBuilderObserveSerieBuildsPayloadLocalDictionaries(t *testing.T) {
	builder := newSegmentShadowBuilder()

	builder.observeSerie(&pkgmetrics.Serie{
		Name:           "custom.metric",
		Tags:           tagset.CompositeTagsFromSlice([]string{"env:test", "service:agent"}),
		Host:           "host-a",
		Device:         "eth0",
		MType:          pkgmetrics.APIGaugeType,
		SourceTypeName: "System",
		Unit:           "byte",
		Points: []pkgmetrics.Point{
			{Ts: 1, Value: 2},
			{Ts: 2, Value: 3},
		},
		Resources: []pkgmetrics.Resource{{Type: "pod", Name: "pod-a"}},
	})

	require.Equal(t, 1, builder.seriesRows)
	require.Equal(t, 2, builder.points)
	require.Contains(t, builder.names, "custom.metric")
	require.Contains(t, builder.tagStrings, "env:test")
	require.Contains(t, builder.tagStrings, "service:agent")
	require.Len(t, builder.tagsets, 1)
	require.Contains(t, builder.sourceTypeNames, "System")
	require.Contains(t, builder.units, "byte")
	require.Contains(t, builder.resourceStrings, "host")
	require.Contains(t, builder.resourceStrings, "host-a")
	require.Contains(t, builder.resourceStrings, "device")
	require.Contains(t, builder.resourceStrings, "eth0")
	require.Contains(t, builder.resourceStrings, "pod")
	require.Contains(t, builder.resourceStrings, "pod-a")
	require.Len(t, builder.resources, 1)
	require.Len(t, builder.origins, 1)
	require.Positive(t, builder.estBytes)
}
