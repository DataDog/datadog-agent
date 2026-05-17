// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"encoding/json"
	"fmt"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultSpotPercentage               = 100 // schedule all pods on spot by default to maximize cost savings.
	defaultMinOnDemandReplicas          = 0   // no minimum by default; override per workload via annotation.
	defaultScheduleTimeout              = 1 * time.Minute
	defaultFallbackDuration             = 2 * time.Minute
	defaultRebalanceStabilizationPeriod = 1 * time.Minute
)

// Config holds the spot scheduler configuration defaults.
type Config struct {
	// Percentage is the default target percentage of spot replicas out of total replicas [0, 100].
	Percentage int
	// MinOnDemandReplicas is the default minimum number of on-demand replicas to keep running
	// regardless of the spot percentage. Must be non-negative.
	MinOnDemandReplicas int
	// ScheduleTimeout is the maximum time to wait for a spot pods to be scheduled
	// before triggering on-demand fallback.
	ScheduleTimeout time.Duration
	// FallbackDuration is how long scheduler stays in on-demand fallback mode
	// after a spot scheduling failure.
	FallbackDuration time.Duration
	// RebalanceStabilizationPeriod is the time between rebalancing
	// decisions for a workload to avoid pod churn.
	RebalanceStabilizationPeriod time.Duration
}

// String returns a human-readable representation of the Config.
func (c Config) String() string {
	return fmt.Sprintf("percentage=%d%%, minOnDemandReplicas=%d, scheduleTimeout=%v, fallbackDuration=%v, rebalanceStabilizationPeriod=%v",
		c.Percentage, c.MinOnDemandReplicas, c.ScheduleTimeout, c.FallbackDuration, c.RebalanceStabilizationPeriod)
}

// ReadConfig reads spot scheduler configuration from cfg, falling back to defaults for out-of-range values.
func ReadConfig(cfg pkgconfigmodel.Reader) Config {
	c := Config{
		Percentage:                   cfg.GetInt("autoscaling.cluster.spot.defaults.percentage"),
		MinOnDemandReplicas:          cfg.GetInt("autoscaling.cluster.spot.defaults.min_on_demand_replicas"),
		ScheduleTimeout:              cfg.GetDuration("autoscaling.cluster.spot.schedule_timeout"),
		FallbackDuration:             cfg.GetDuration("autoscaling.cluster.spot.fallback_duration"),
		RebalanceStabilizationPeriod: cfg.GetDuration("autoscaling.cluster.spot.rebalance_stabilization_period"),
	}

	if c.Percentage < 0 || c.Percentage > 100 {
		log.Warnf("autoscaling.cluster.spot.defaults.percentage=%d is out of range [0, 100], using default %d", c.Percentage, defaultSpotPercentage)
		c.Percentage = defaultSpotPercentage
	}
	if c.MinOnDemandReplicas < 0 {
		log.Warnf("autoscaling.cluster.spot.defaults.min_on_demand_replicas=%d is negative, using default %d", c.MinOnDemandReplicas, defaultMinOnDemandReplicas)
		c.MinOnDemandReplicas = defaultMinOnDemandReplicas
	}
	if c.ScheduleTimeout <= 0 {
		log.Warnf("autoscaling.cluster.spot.schedule_timeout=%v is not positive, using default %v", c.ScheduleTimeout, defaultScheduleTimeout)
		c.ScheduleTimeout = defaultScheduleTimeout
	}
	if c.FallbackDuration <= 0 {
		log.Warnf("autoscaling.cluster.spot.fallback_duration=%v is not positive, using default %v", c.FallbackDuration, defaultFallbackDuration)
		c.FallbackDuration = defaultFallbackDuration
	}
	if c.RebalanceStabilizationPeriod <= 0 {
		log.Warnf("autoscaling.cluster.spot.rebalance_stabilization_period=%v is not positive, using default %v", c.RebalanceStabilizationPeriod, defaultRebalanceStabilizationPeriod)
		c.RebalanceStabilizationPeriod = defaultRebalanceStabilizationPeriod
	}
	return c
}

// workloadSpotConfig holds per-workload spot scheduling parameters.
type workloadSpotConfig struct {
	percentage    int
	minOnDemand   int
	disabledUntil time.Time
}

// isDisabled returns true if spot scheduling is disabled at time now.
func (c workloadSpotConfig) isDisabled(now time.Time) bool {
	return now.Before(c.disabledUntil)
}

// spotConfigAnnotation represents the JSON structure for per-workload spot configuration.
type spotConfigAnnotation struct {
	Percentage          *int `json:"percentage,omitempty"`
	MinOnDemandReplicas *int `json:"minOnDemandReplicas,omitempty"`
}

// overrideFromAnnotations overrides cfg fields from spot annotations, leaving unset fields unchanged.
func overrideFromAnnotations(cfg *workloadSpotConfig, annotations map[string]string) {
	if v := annotations[SpotConfigAnnotation]; v != "" {
		var ac spotConfigAnnotation
		if err := json.Unmarshal([]byte(v), &ac); err != nil {
			log.Warnf("Failed to parse %s annotation: %v", SpotConfigAnnotation, err)
		} else {
			if ac.Percentage != nil && *ac.Percentage >= 0 && *ac.Percentage <= 100 {
				cfg.percentage = *ac.Percentage
			}
			if ac.MinOnDemandReplicas != nil && *ac.MinOnDemandReplicas >= 0 {
				cfg.minOnDemand = *ac.MinOnDemandReplicas
			}
		}
	}
	if v := annotations[SpotDisabledUntilAnnotation]; v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			cfg.disabledUntil = t
		} else {
			cfg.disabledUntil = time.Time{}
		}
	}
}
