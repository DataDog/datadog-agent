// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMetricSampleOwnsMetadata(t *testing.T) {
	tags := []string{"source:nvml"}
	workloads := []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "1"}}
	metric := NewMetric("utilization", 1, ddmetrics.GaugeType, Low, tags, workloads)

	tags[0] = "source:changed"
	workloads[0].ID = "2"
	require.Equal(t, []string{"source:nvml"}, metric.Tags())
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)

	returnedTags := metric.Tags()
	returnedTags[0] = "source:returned"
	require.Equal(t, []string{"source:nvml"}, metric.Tags())

	returnedWorkloads := metric.AssociatedWorkloads()
	returnedWorkloads[0].ID = "3"
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)

	clone, ok := metric.Clone().(*Metric)
	require.True(t, ok)
	clone.AppendTags([]string{"scope:clone"})
	clone.associatedWorkloads[0].ID = "4"

	require.Equal(t, []string{"source:nvml"}, metric.Tags())
	require.Equal(t, "1", metric.AssociatedWorkloads()[0].ID)
}
