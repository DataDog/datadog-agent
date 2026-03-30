// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
)

func TestReadConfig(t *testing.T) {
	t.Run("valid values returned as-is", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.cluster.spot.percentage":                     60,
			"autoscaling.cluster.spot.min_on_demand_replicas":         3,
			"autoscaling.cluster.spot.schedule_timeout":               "30s",
			"autoscaling.cluster.spot.fallback_duration":              "5m",
			"autoscaling.cluster.spot.rebalance_stabilization_period": "2m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 60, c.Percentage)
		assert.Equal(t, 3, c.MinOnDemandReplicas)
		assert.Equal(t, 30*time.Second, c.ScheduleTimeout)
		assert.Equal(t, 5*time.Minute, c.FallbackDuration)
		assert.Equal(t, 2*time.Minute, c.RebalanceStabilizationPeriod)
	})

	t.Run("percentage out of range falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.cluster.spot.percentage":             101,
			"autoscaling.cluster.spot.min_on_demand_replicas": 1,
			"autoscaling.cluster.spot.schedule_timeout":       "30s",
			"autoscaling.cluster.spot.fallback_duration":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 100, c.Percentage)
	})

	t.Run("negative min_on_demand_replicas falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.cluster.spot.percentage":             50,
			"autoscaling.cluster.spot.min_on_demand_replicas": -1,
			"autoscaling.cluster.spot.schedule_timeout":       "30s",
			"autoscaling.cluster.spot.fallback_duration":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 3, c.MinOnDemandReplicas)
	})

	t.Run("zero durations fall back to defaults", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.cluster.spot.percentage":                     50,
			"autoscaling.cluster.spot.min_on_demand_replicas":         1,
			"autoscaling.cluster.spot.schedule_timeout":               "0s",
			"autoscaling.cluster.spot.fallback_duration":              "0s",
			"autoscaling.cluster.spot.rebalance_stabilization_period": "0s",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 1*time.Minute, c.ScheduleTimeout)
		assert.Equal(t, 2*time.Minute, c.FallbackDuration)
		assert.Equal(t, 1*time.Minute, c.RebalanceStabilizationPeriod)
	})
}
