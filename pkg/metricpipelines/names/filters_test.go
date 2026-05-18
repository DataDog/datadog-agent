// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestLoadOutsideCloudCostOnly(t *testing.T) {
	cfg := map[string]interface{}{
		"infrastructure_mode": "full",
		"metric_filterlist":   []string{"blocked.metric"},
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.True(t, filters.ShouldDrop("blocked.metric"))
	assert.False(t, filters.ShouldDrop("system.cpu.user"))
}

func TestLoadCloudCostOnly(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist":                                []string{"blocked.metric"},
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics":              []string{"system.cpu"},
		"integration.cloud_cost_only.metrics_match_prefix": true,
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.False(t, filters.ShouldDrop("system.cpu.user"))
	assert.True(t, filters.ShouldDrop("system.disk.free"))
	assert.True(t, filters.ShouldDrop("blocked.metric"), "metric_filterlist and allowlist both apply in cloud_cost_only mode")
}

func TestLoadCloudCostOnlyExplicitBlock(t *testing.T) {
	cfg := map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics_blocked":      []string{"system.disk"},
		"integration.cloud_cost_only.metrics_match_prefix": true,
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.True(t, filters.ShouldDrop("system.disk.free"))
	assert.False(t, filters.ShouldDrop("system.cpu.user"))
}
