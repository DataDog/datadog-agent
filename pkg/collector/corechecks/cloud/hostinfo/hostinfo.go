// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostinfo implements a check that collects host-local information
// from cloud provider metadata services (e.g., preemption events for spot instances).
package hostinfo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/shirou/gopsutil/v4/host"
)

// CheckName is the name of the check
const CheckName = "cloud_hostinfo"

// PreemptionEventType is the event type for preemption events
const PreemptionEventType = "HostPreemption"

// Check collects host information from cloud provider metadata services
type Check struct {
	core.CheckBase

	cloudProvider         string
	cloudProviderOnce     sync.Once
	terminationTime       time.Time
	preemptionUnsupported bool // Set to true when preemption detection is not supported or instance is not preemptible
}

// For testing purposes
var (
	detectCloudProviderFn      = cloudproviders.DetectCloudProvider
	getPreemptionTerminationFn = cloudproviders.GetPreemptionTerminationTime
	uptime                     = host.Uptime
)

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}

	// Check for preemption events (e.g., AWS Spot, GCE Preemptible, Azure Spot)
	c.checkPreemptionEvents(sender)

	sender.Commit()

	return nil
}

// checkPreemptionEvents checks for scheduled preemption termination and emits an event if found
func (c *Check) checkPreemptionEvents(sender sender.Sender) {
	ctx := context.Background()

	// Detect cloud provider once, avoid blocking `Configure()`
	c.cloudProviderOnce.Do(func() {
		c.cloudProvider, _ = detectCloudProviderFn(ctx, false)
	})
	if c.cloudProvider == "" {
		return
	}

	// If preemption is unsupported for this instance, don't poll
	if c.preemptionUnsupported {
		return
	}

	// If termination time is already set, don't check again
	if !c.terminationTime.IsZero() {
		return
	}

	terminationTime, err := getPreemptionTerminationFn(ctx, c.cloudProvider)
	if err != nil {
		// Check if we should stop polling for preemption events
		if errors.Is(err, cloudproviders.ErrNotPreemptible) || errors.Is(err, cloudproviders.ErrPreemptionUnsupported) {
			c.preemptionUnsupported = true
			log.Debugf("Preemption detection disabled, cloud provider: %s, error: %s", c.cloudProvider, err)
		}
		log.Tracef("Preemption detection returned an error (usually expected), cloud provider: %s, error: %s", c.cloudProvider, err)
		return
	}

	// Store termination time to avoid emitting duplicate events
	c.terminationTime = terminationTime

	// If termination time is in the past, don't emit event
	timeUntilTermination := int(math.Ceil(time.Until(terminationTime).Seconds()))
	if timeUntilTermination < 0 {
		return
	}

	// Get current uptime for the event text
	uptimeSeconds, err := uptime()
	if err != nil {
		log.Warnf("Could not retrieve uptime to enrich preemption event, error: %s", err)
		uptimeSeconds = 0
	}

	// Emit event for preemption termination
	ev := event.Event{
		Title:          "Instance Preemption",
		Text:           fmt.Sprintf("This instance is scheduled for preemption at: %s, uptime: %d seconds", terminationTime.UTC().Format(time.RFC3339), uptimeSeconds+uint64(timeUntilTermination)),
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeInfo,
		SourceTypeName: "system",
		EventType:      PreemptionEventType,
	}
	sender.Event(ev)
	log.Infof("Instance preemption detected, will terminate at: %s", terminationTime.UTC().Format(time.RFC3339))
}
