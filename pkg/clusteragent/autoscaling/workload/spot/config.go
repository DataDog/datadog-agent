// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"fmt"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultSpotPercentage       = 50
	defaultMinOnDemandReplicas  = 1
	defaultSpotScheduleTimeout  = 1 * time.Minute
	defaultSpotDisabledInterval = 2 * time.Minute
)

// Config holds the spot scheduler configuration defaults.
// These values serve as defaults when a workload doesn't specify the corresponding annotation.
type Config struct {
	Percentage          int
	MinOnDemandReplicas int
	ScheduleTimeout     time.Duration
	DisabledInterval    time.Duration
}

// String returns a human-readable representation of the Config.
func (c Config) String() string {
	return fmt.Sprintf("SpotConfig{percentage=%d%%, minOnDemandReplicas=%d, scheduleTimeout=%v, disabledInterval=%v}",
		c.Percentage, c.MinOnDemandReplicas, c.ScheduleTimeout, c.DisabledInterval)
}

// ReadConfig reads spot scheduler configuration from cfg, falling back to defaults for out-of-range values.
func ReadConfig(cfg pkgconfigmodel.Reader) Config {
	c := Config{
		Percentage:          cfg.GetInt("autoscaling.workload.spot.percentage"),
		MinOnDemandReplicas: cfg.GetInt("autoscaling.workload.spot.min_on_demand_replicas"),
		ScheduleTimeout:     cfg.GetDuration("autoscaling.workload.spot.schedule_timeout"),
		DisabledInterval:    cfg.GetDuration("autoscaling.workload.spot.disabled_interval"),
	}

	if c.Percentage < 0 || c.Percentage > 100 {
		log.Warnf("autoscaling.workload.spot.percentage=%d is out of range [0, 100], using default %d", c.Percentage, defaultSpotPercentage)
		c.Percentage = defaultSpotPercentage
	}
	if c.MinOnDemandReplicas < 0 {
		log.Warnf("autoscaling.workload.spot.min_on_demand_replicas=%d is negative, using default %d", c.MinOnDemandReplicas, defaultMinOnDemandReplicas)
		c.MinOnDemandReplicas = defaultMinOnDemandReplicas
	}
	if c.ScheduleTimeout <= 0 {
		log.Warnf("autoscaling.workload.spot.schedule_timeout=%v is not positive, using default %v", c.ScheduleTimeout, defaultSpotScheduleTimeout)
		c.ScheduleTimeout = defaultSpotScheduleTimeout
	}
	if c.DisabledInterval <= 0 {
		log.Warnf("autoscaling.workload.spot.disabled_interval=%v is not positive, using default %v", c.DisabledInterval, defaultSpotDisabledInterval)
		c.DisabledInterval = defaultSpotDisabledInterval
	}
	return c
}
