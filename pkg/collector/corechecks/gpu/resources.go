// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	gpuresources "github.com/DataDog/datadog-agent/pkg/gpu/resources"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func (c *Check) configureResources() error {
	c.resourcePublisher = nil
	if !c.gpuConfig.Resources.Enabled {
		return nil
	}
	options := gpuresources.Options{
		TTL: c.gpuConfig.Resources.TTL,
	}
	if options.TTL <= 0 {
		return errors.New("gpu.resources.ttl must be positive")
	}
	if c.Interval() > time.Duration(math.MaxInt64/3) {
		return errors.New("GPU collection interval is too large for resource retention")
	}
	if minimumTTL := 3 * c.Interval(); options.TTL < minimumTTL {
		_ = c.Warnf("gpu.resources.ttl increased from %s to %s to cover three collection intervals", options.TTL, minimumTTL)
		options.TTL = minimumTTL
	}
	var err error
	c.resourcePublisher, err = gpuresources.NewPublisher(options)
	return err
}

func (c *Check) publishResources(snd sender.Sender, now time.Time) error {
	if c.resourcePublisher == nil {
		return nil
	}
	// Get reads the Agent's hostname cache, including canonical hostname updates.
	host, err := hostname.Get(context.Background())
	if err != nil {
		return fmt.Errorf("GPU resource hostname resolution failed: %w", err)
	}
	devices, err := c.deviceCache.AllPhysicalDevices()
	if err != nil {
		return fmt.Errorf("GPU resource enumeration failed: %w", err)
	}
	return c.resourcePublisher.Publish(now, host, gpuresources.Snapshot(devices), snd)
}
