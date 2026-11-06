// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestEndpointMetricFilterGrouping(t *testing.T) {
	filters := CompileEndpointMetricFilters(configutils.MetricsEndpointRouting{
		MetricAllowlist:       map[string][]string{"a": {"critical.slo", "billing.latency", "critical.slo"}, "b": {"critical.slo"}},
		MetricPrefixAllowlist: map[string][]string{"a": {"billing.latency", "billing.", "billing."}, "b": {"billing."}, "all": {""}},
	})
	require.Same(t, filters["a"], filters["b"])
	require.True(t, filters["all"].Filter(&metrics.Serie{Name: "any.metric"}))
}
