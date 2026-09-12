// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
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

func TestEventHasUniqueKeyAndOwnsMetadata(t *testing.T) {
	timestamp := time.Unix(100, 0)
	tags := []string{"source:kmsg"}
	workloads := []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "1"}}
	payload := event.Event{Title: "XID 31 error", Tags: []string{"event_tag:value"}}

	first := NewEvent(payload, timestamp, Medium, tags, workloads)
	second := NewEvent(payload, timestamp, Medium, tags, workloads)

	require.NotEqual(t, first.Key(), second.Key())

	tags[0] = "source:changed"
	workloads[0].ID = "2"
	payload.Tags[0] = "event_tag:changed"
	require.Equal(t, []string{"source:kmsg"}, first.Tags())
	require.Equal(t, "1", first.AssociatedWorkloads()[0].ID)
	require.Equal(t, []string{"event_tag:value"}, first.event.Tags)

	clone, ok := first.Clone().(*Event)
	require.True(t, ok)
	clone.AppendTags([]string{"scope:clone"})
	clone.event.Tags[0] = "event_tag:clone"

	require.Equal(t, []string{"source:kmsg"}, first.Tags())
	require.Equal(t, []string{"event_tag:value"}, first.event.Tags)
	require.Equal(t, first.Key(), clone.Key())
}
