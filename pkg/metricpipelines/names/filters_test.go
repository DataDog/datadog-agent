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
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

func TestLoadOutsideCloudCostOnly(t *testing.T) {
	cfg := map[string]interface{}{
		"infrastructure_mode": "full",
		"metric_filterlist":   []string{"blocked.metric"},
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.True(t, filters.ShouldDrop(FilterContext{Name: "blocked.metric"}))
	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
}

func TestFiltersExclusiveAllowlist(t *testing.T) {
	allowList := utilstrings.NewAllowlistMatcher([]string{"system.cpu"}, true)
	filters := NewTestFilters(CriterionMetricFilterList, utilstrings.Matcher{}, allowList)

	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.disk.free"}))
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

	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.disk.free"}))
	assert.True(t, filters.ShouldDrop(FilterContext{Name: "blocked.metric"}),
		"metric_filterlist and cloud_cost blocklist both apply in cloud_cost_only mode")
}

func TestLoadCloudCostOnlyExplicitEmptyMetricsDeniesAll(t *testing.T) {
	cfg := map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics":              []string{},
		"integration.cloud_cost_only.metrics_match_prefix": true,
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.mem.pct_usable"}))
	assert.True(t, filters.ShouldDrop(FilterContext{
		Name:   "system.disk.free",
		Source: metrics.MetricSourceDisk,
	}))
	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:          "custom.metric",
		FromDogstatsd: true,
	}))
}

func TestLoadCloudCostOnlyUnsetMetricsUsesDefaults(t *testing.T) {
	filters := loadCloudCostFilters(t, map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics_match_prefix": true,
	})

	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.mem.pct_usable"}))
	assert.True(t, filters.ShouldDrop(FilterContext{
		Name:   "system.disk.free",
		Source: metrics.MetricSourceDisk,
	}))
}

func TestLoadCloudCostOnlyExplicitEmptyMetricsDeniesAllFromYAML(t *testing.T) {
	configComponent := config.NewMockFromYAML(t, `
infrastructure_mode: cloud_cost_only
integration:
  cloud_cost_only:
    metrics: []
`)
	assert.True(t, configComponent.IsConfigured("integration.cloud_cost_only.metrics"))

	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))
	filters := Load(configComponent, fl)

	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:          "e2e.custom",
		FromDogstatsd: true,
	}))
}

func TestLoadCloudCostOnlyCustomAllowlistExactMatch(t *testing.T) {
	filters := loadCloudCostFilters(t, map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics":              []string{"system.mem.pct_usable"},
		"integration.cloud_cost_only.metrics_match_prefix": false,
	})

	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.mem.pct_usable"}))
	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.mem.used"}))
	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
}

func TestLoadCloudCostOnlyMetricsBlockedOverridesDefaultAllowlist(t *testing.T) {
	filters := loadCloudCostFilters(t, map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics_blocked":      []string{"system.cpu"},
		"integration.cloud_cost_only.metrics_match_prefix": true,
	})

	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.net.bytes_rcvd"}))
}

func TestLoadCloudCostOnlyExplicitEmptyCustomCheckBypassed(t *testing.T) {
	filters := loadCloudCostFilters(t, map[string]interface{}{
		"infrastructure_mode":                 "cloud_cost_only",
		"integration.cloud_cost_only.metrics": []string{},
	})

	assert.True(t, filters.ShouldDrop(FilterContext{
		Name:   "system.mem.pct_usable",
		Source: metrics.MetricSourceMemory,
	}))
	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:      "my.metric",
		CheckName: "custom_my_check",
	}))
}

func TestCloudCostMetricsCriterionMatchers(t *testing.T) {
	c := cloudCostMetricsCriterion{}

	t.Run("unset metrics uses defaults", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{
			"integration.cloud_cost_only.metrics_match_prefix": true,
		})
		assert.False(t, cfg.IsConfigured("integration.cloud_cost_only.metrics"))

		_, allow := c.matchers(cfg, nil)
		assert.False(t, allow.ShouldDrop("system.mem.pct_usable"))
		assert.True(t, allow.ShouldDrop("system.disk.free"))
	})

	t.Run("explicit empty deny all", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{
			"integration.cloud_cost_only.metrics":              []string{},
			"integration.cloud_cost_only.metrics_match_prefix": true,
		})
		assert.True(t, cfg.IsConfigured("integration.cloud_cost_only.metrics"))

		_, allow := c.matchers(cfg, nil)
		assert.True(t, allow.ShouldDrop("system.mem.pct_usable"))
		assert.True(t, allow.ShouldDrop("system.cpu.user"))
	})

	t.Run("explicit list", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{
			"integration.cloud_cost_only.metrics":              []string{"system.cpu"},
			"integration.cloud_cost_only.metrics_match_prefix": true,
		})

		_, allow := c.matchers(cfg, nil)
		assert.False(t, allow.ShouldDrop("system.cpu.user"))
		assert.True(t, allow.ShouldDrop("system.disk.free"))
	})
}

func loadCloudCostFilters(t *testing.T, overrides map[string]interface{}) Filters {
	t.Helper()
	configComponent := config.NewMockWithOverrides(t, overrides)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))
	return Load(configComponent, fl)
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

	assert.True(t, filters.ShouldDrop(FilterContext{Name: "system.disk.free"}))
	assert.False(t, filters.ShouldDrop(FilterContext{Name: "system.cpu.user"}))
}

func TestLoadCloudCostOnlyForwardsBypassMetrics(t *testing.T) {
	cfg := map[string]interface{}{
		"infrastructure_mode":                              "cloud_cost_only",
		"integration.cloud_cost_only.metrics":              []string{"system.cpu"},
		"integration.cloud_cost_only.metrics_match_prefix": true,
		"integration.additional":                           []string{"my_additional_check"},
	}
	configComponent := config.NewMockWithOverrides(t, cfg)
	fl := filterlistimpl.NewFilterList(logmock.New(t), configComponent, fxutil.Test[telemetry.Component](t, telemetrynoop.Module()))

	filters := Load(configComponent, fl)

	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:          "custom.my.metric",
		FromDogstatsd: true,
	}))
	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:      "custom.my.metric",
		CheckName: "custom_my_check",
	}))
	assert.False(t, filters.ShouldDrop(FilterContext{
		Name:      "additional.metric",
		CheckName: "my_additional_check",
	}))
	assert.True(t, filters.ShouldDrop(FilterContext{
		Name:      "system.disk.free",
		CheckName: "disk",
		Source:    metrics.MetricSourceDisk,
	}))
}

func TestShouldDropCloudCost(t *testing.T) {
	blockList := utilstrings.NewBlocklistMatcher([]string{"blocked"}, true)
	allowList := utilstrings.NewAllowlistMatcher([]string{"system.cpu"}, true)

	t.Run("blocklist wins", func(t *testing.T) {
		assert.True(t, shouldDropCloudCost(FilterContext{Name: "blocked.metric"}, blockList, allowList, nil))
	})
	t.Run("dogstatsd forwarded", func(t *testing.T) {
		assert.False(t, shouldDropCloudCost(FilterContext{
			Name:          "not.on.list",
			FromDogstatsd: true,
		}, blockList, allowList, nil))
	})
	t.Run("jmx over dogstatsd forwarded", func(t *testing.T) {
		assert.False(t, shouldDropCloudCost(FilterContext{
			Name:          "kafka.metric",
			Source:        metrics.MetricSourceKafka,
			CheckName:     "kafka",
			FromDogstatsd: true,
		}, blockList, allowList, nil))
	})
	t.Run("allowlist forwarded", func(t *testing.T) {
		assert.False(t, shouldDropCloudCost(FilterContext{Name: "system.cpu.user"}, blockList, allowList, nil))
	})
	t.Run("integration dropped", func(t *testing.T) {
		assert.True(t, shouldDropCloudCost(FilterContext{
			Name:   "system.disk.free",
			Source: metrics.MetricSourceDisk,
		}, blockList, allowList, nil))
	})
}

func TestFilterContextBypassesCloudCostFilter(t *testing.T) {
	assert.True(t, FilterContext{FromDogstatsd: true}.BypassesCloudCostFilter(nil))
	assert.True(t, FilterContext{
		CheckName:     "kafka",
		Source:        metrics.MetricSourceKafka,
		FromDogstatsd: true,
	}.BypassesCloudCostFilter(nil))
	assert.True(t, FilterContext{CheckName: "custom_foo"}.BypassesCloudCostFilter(nil))
	assert.True(t, FilterContext{CheckName: "extra"}.BypassesCloudCostFilter([]string{"extra"}))
	assert.False(t, FilterContext{CheckName: "disk", Source: metrics.MetricSourceDisk}.BypassesCloudCostFilter(nil))
	assert.False(t, FilterContext{CheckName: "kafka", Source: metrics.MetricSourceKafka}.BypassesCloudCostFilter(nil))
}
