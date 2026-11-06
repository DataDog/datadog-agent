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
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/shirou/gopsutil/v4/host"
)

// CheckName is the name of the check
const CheckName = "cloud_hostinfo"

// PreemptionEventType is the event type for preemption events
const PreemptionEventType = "HostPreemption"

// PreemptionRiskEventType is the event type for rebalance recommendation events
const PreemptionRiskEventType = "HostPreemptionRisk"

// After this many consecutive failed IMDS lookups, the check only retries
// once per metadataFailureBackoff instead of on every run: on hosts where
// IMDS can never answer (e.g. hop-limited containers), every-run lookups
// each block a collector slot for the metadata timeout doing work that
// cannot succeed (#55269).
const (
	metadataFailureThreshold = 4
	metadataFailureBackoff   = 60 * time.Second
)

// metadataLookupFailure reports whether an error from a metadata lookup is a
// connectivity-style failure that can never succeed on this host, as opposed
// to the healthy no-active-notice response: IMDS answers 404 for the spot
// termination and rebalance endpoints when no notice exists, and those must
// not count toward the backoff.
func metadataLookupFailure(err error) bool {
	if err == nil {
		return false
	}
	var statusCodeError *httputils.StatusCodeError
	if errors.As(err, &statusCodeError) {
		return statusCodeError.StatusCode != http.StatusNotFound
	}
	return true
}

// Check collects host information from cloud provider metadata services
type Check struct {
	core.CheckBase

	cloudProvider         string
	cloudProviderOnce     sync.Once
	terminationTime       time.Time
	rebalanceNoticeTime   time.Time
	preemptionUnsupported bool // Set to true when preemption detection is not supported or instance is not preemptible

	metadataFailures    int       // consecutive transient preemption-lookup failures
	metadataLastAttempt time.Time // time of the last preemption lookup
}

// For testing purposes
var (
	detectCloudProviderFn        = cloudproviders.DetectCloudProvider
	getPreemptionTerminationFn   = cloudproviders.GetPreemptionTerminationTime
	getRebalanceRecommendationFn = cloudproviders.GetRebalanceRecommendationTime
	uptime                       = host.Uptime
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
	c.checkRebalanceRecommendation(sender)

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

	// After repeated transient failures (e.g. IMDS unreachable from a
	// hop-limited container), retry at most once per backoff window instead
	// of on every check run.
	if c.metadataFailures >= metadataFailureThreshold && time.Since(c.metadataLastAttempt) < metadataFailureBackoff {
		log.Tracef("Skipping preemption detection, %d consecutive failures, backing off until %s", c.metadataFailures, c.metadataLastAttempt.Add(metadataFailureBackoff))
		return
	}

	terminationTime, err := getPreemptionTerminationFn(ctx, c.cloudProvider)
	if err != nil {
		// Check if we should stop polling for preemption events
		if errors.Is(err, cloudproviders.ErrNotPreemptible) || errors.Is(err, cloudproviders.ErrPreemptionUnsupported) {
			c.preemptionUnsupported = true
			log.Debugf("Preemption detection disabled, cloud provider: %s, error: %s", c.cloudProvider, err)
			return
		}
		// The healthy no-active-notice 404 must not count toward backoff.
		if metadataLookupFailure(err) {
			c.metadataFailures++
			c.metadataLastAttempt = time.Now()
		}
		log.Tracef("Preemption detection returned an error (usually expected), cloud provider: %s, error: %s", c.cloudProvider, err)
		return
	}
	c.metadataFailures = 0

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

// checkRebalanceRecommendation checks for an AWS rebalance recommendation and emits an event if found.
// Skipped when a termination time is already known (termination supersedes the recommendation).
func (c *Check) checkRebalanceRecommendation(sender sender.Sender) {
	if c.cloudProvider == "" || c.preemptionUnsupported {
		return
	}

	// If termination is already scheduled, a rebalance recommendation is redundant
	if !c.terminationTime.IsZero() {
		return
	}

	// If rebalance recommendation was already emitted, don't emit again
	if !c.rebalanceNoticeTime.IsZero() {
		return
	}

	// The rebalance lookup hits the same unreachable IMDS endpoint; back it
	// off on the same counter as the preemption lookup.
	if c.metadataFailures >= metadataFailureThreshold && time.Since(c.metadataLastAttempt) < metadataFailureBackoff {
		log.Tracef("Skipping rebalance recommendation check, %d consecutive failures, backing off until %s", c.metadataFailures, c.metadataLastAttempt.Add(metadataFailureBackoff))
		return
	}

	noticeTime, err := getRebalanceRecommendationFn(context.Background(), c.cloudProvider)
	if err != nil {
		if metadataLookupFailure(err) {
			c.metadataFailures++
			c.metadataLastAttempt = time.Now()
		}
		log.Tracef("Rebalance recommendation check returned an error (usually expected), cloud provider: %s, error: %s", c.cloudProvider, err)
		return
	}
	c.metadataFailures = 0

	c.rebalanceNoticeTime = noticeTime

	// Get current uptime for the event text
	uptimeSeconds, err := uptime()
	if err != nil {
		log.Warnf("Could not retrieve uptime to enrich preemption event, error: %s", err)
		uptimeSeconds = 0
	}

	ev := event.Event{
		Title:          "Elevated risk of Instance Preemption",
		Text:           fmt.Sprintf("This instance received a rebalance recommendation event (elevated risk of preemption) at: %s, uptime: %d seconds", noticeTime.UTC().Format(time.RFC3339), uptimeSeconds),
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeInfo,
		SourceTypeName: "system",
		EventType:      PreemptionRiskEventType,
	}
	sender.Event(ev)
	log.Infof("Instance rebalance recommendation detected at: %s", noticeTime.UTC().Format(time.RFC3339))
}
