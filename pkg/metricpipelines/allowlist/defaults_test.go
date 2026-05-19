// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package allowlist_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metricpipelines/allowlist"
)

func TestDefaultCloudCostMetricsMatchesAgentConfig(t *testing.T) {
	cfg := configmock.New(t)
	assert.Equal(t, allowlist.DefaultCloudCostMetrics,
		cfg.GetStringSlice("integration.cloud_cost_only.metrics"),
		"keep allowlist.DefaultCloudCostMetrics in sync with pkg/config/setup/common_settings.go")
}
