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
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/spot"
)

func TestReadConfig(t *testing.T) {
	t.Run("valid values returned as-is", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.workload.spot.percentage":             60,
			"autoscaling.workload.spot.min_on_demand_replicas": 3,
			"autoscaling.workload.spot.schedule_timeout":       "30s",
			"autoscaling.workload.spot.disabled_interval":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 60, c.Percentage)
		assert.Equal(t, 3, c.MinOnDemandReplicas)
		assert.Equal(t, 30*time.Second, c.ScheduleTimeout)
		assert.Equal(t, 5*time.Minute, c.DisabledInterval)
	})

	t.Run("percentage out of range falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.workload.spot.percentage":             101,
			"autoscaling.workload.spot.min_on_demand_replicas": 1,
			"autoscaling.workload.spot.schedule_timeout":       "30s",
			"autoscaling.workload.spot.disabled_interval":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 50, c.Percentage)
	})

	t.Run("negative min_on_demand_replicas falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.workload.spot.percentage":             50,
			"autoscaling.workload.spot.min_on_demand_replicas": -1,
			"autoscaling.workload.spot.schedule_timeout":       "30s",
			"autoscaling.workload.spot.disabled_interval":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 1, c.MinOnDemandReplicas)
	})

	t.Run("zero schedule_timeout falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.workload.spot.percentage":             50,
			"autoscaling.workload.spot.min_on_demand_replicas": 1,
			"autoscaling.workload.spot.schedule_timeout":       "0s",
			"autoscaling.workload.spot.disabled_interval":      "5m",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 1*time.Minute, c.ScheduleTimeout)
	})

	t.Run("zero disabled_interval falls back to default", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"autoscaling.workload.spot.percentage":             50,
			"autoscaling.workload.spot.min_on_demand_replicas": 1,
			"autoscaling.workload.spot.schedule_timeout":       "30s",
			"autoscaling.workload.spot.disabled_interval":      "0s",
		})
		c := spot.ReadConfig(cfg)
		assert.Equal(t, 2*time.Minute, c.DisabledInterval)
	})
}
