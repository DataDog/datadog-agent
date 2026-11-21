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

// After this many consecutive failed IMDS polling runs, the check only retries
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

// metadataOutcome is the result of one metadata lookup within a Run().
type metadataOutcome int

const (
	// metadataNotAttempted: the lookup was skipped (no cloud provider,
	// preemption unsupported, notice already known, or backoff) or stopped
	// on a sentinel error; it says nothing about IMDS reachability.
	metadataNotAttempted metadataOutcome = iota
	// metadataAnswered: IMDS answered, either with a value or with the
	// healthy no-active-notice 404.
	metadataAnswered
	// metadataFailed: the lookup failed in a way that counts toward the
	// backoff (see metadataLookupFailure).
	metadataFailed
)

// metadataOutcomeFromError classifies a metadata lookup result.
func metadataOutcomeFromError(err error) metadataOutcome {
	if metadataLookupFailure(err) {
		return metadataFailed
	}
	return metadataAnswered
}

// updateMetadataBackoff updates the shared failure counter once per Run(),
// from the outcomes of the lookups attempted in that run. Preemption and
// rebalance hit the same metadata service, so their outcomes are combined
// before touching the counter: otherwise an outage where both fail would
// count twice per run and halve the effective metadataFailureThreshold.
//
// If any lookup got an answer, IMDS is reachable and the counter is reset even
// if the other lookup failed. This preserves healthy polling at the cost of
// retrying the failing lookup (and potentially waiting out its timeout) on
// every run. Only runs where every attempted lookup fails count once toward
// the backoff.
func (c *Check) updateMetadataBackoff(outcomes ...metadataOutcome) {
	failed := false
	for _, outcome := range outcomes {
		switch outcome {
		case metadataAnswered:
			c.metadataFailures = 0
			return
		case metadataFailed:
			failed = true
		}
	}
	if !failed {
		return
	}
	c.metadataFailures++
	c.metadataLastFailure = time.Now()
	if c.metadataFailures == metadataFailureThreshold {
		log.Debugf("Cloud metadata lookups failed on %d consecutive runs, retrying preemption and rebalance detection at most once every %s until the metadata service answers", c.metadataFailures, metadataFailureBackoff)
	}
}

// inMetadataBackoff reports whether metadata lookups should be skipped on this
// run: after metadataFailureThreshold consecutive failed runs, lookups are only
// retried once metadataFailureBackoff has elapsed since the last failure.
func (c *Check) inMetadataBackoff() bool {
	return c.metadataFailures >= metadataFailureThreshold && time.Since(c.metadataLastFailure) < metadataFailureBackoff
}

// Check collects host information from cloud provider metadata services
type Check struct {
	core.CheckBase

	cloudProvider         string
	cloudProviderOnce     sync.Once
	terminationTime       time.Time
	rebalanceNoticeTime   time.Time
	preemptionUnsupported bool // Set to true when preemption detection is not supported or instance is not preemptible

	metadataFailures    int       // consecutive polling runs where all attempted metadata lookups failed
	metadataLastFailure time.Time // time of the last polling run where all attempted metadata lookups failed
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
	preemptionOutcome := c.checkPreemptionEvents(sender)
	rebalanceOutcome := c.checkRebalanceRecommendation(sender)
	c.updateMetadataBackoff(preemptionOutcome, rebalanceOutcome)

	sender.Commit()

	return nil
}

// checkPreemptionEvents checks for scheduled preemption termination and emits an event if found.
// It reports the outcome of its metadata lookup, if one was attempted.
func (c *Check) checkPreemptionEvents(sender sender.Sender) metadataOutcome {
	ctx := context.Background()

	// Detect cloud provider once, avoid blocking `Configure()`
	c.cloudProviderOnce.Do(func() {
		c.cloudProvider, _ = detectCloudProviderFn(ctx, false)
	})
	if c.cloudProvider == "" {
		return metadataNotAttempted
	}

	// If preemption is unsupported for this instance, don't poll
	if c.preemptionUnsupported {
		return metadataNotAttempted
	}

	// If termination time is already set, don't check again
	if !c.terminationTime.IsZero() {
		return metadataNotAttempted
	}

	// After repeated transient failures (e.g. IMDS unreachable from a
	// hop-limited container), retry at most once per backoff window instead
	// of on every check run.
	if c.inMetadataBackoff() {
		log.Tracef("Skipping preemption detection, %d consecutive failures, backing off until %s", c.metadataFailures, c.metadataLastFailure.Add(metadataFailureBackoff))
		return metadataNotAttempted
	}

	terminationTime, err := getPreemptionTerminationFn(ctx, c.cloudProvider)
	if err != nil {
		// Check if we should stop polling for preemption events
		if errors.Is(err, cloudproviders.ErrNotPreemptible) || errors.Is(err, cloudproviders.ErrPreemptionUnsupported) {
			c.preemptionUnsupported = true
			log.Debugf("Preemption detection disabled, cloud provider: %s, error: %s", c.cloudProvider, err)
			return metadataNotAttempted
		}
		// The healthy no-active-notice 404 is reported as answered, which
		// resets the counter rather than just skipping the increment: IMDS
		// answered, so the host isn't in the unreachable state the backoff
		// guards against.
		log.Tracef("Preemption detection returned an error (usually expected), cloud provider: %s, error: %s", c.cloudProvider, err)
		return metadataOutcomeFromError(err)
	}

	// Store termination time to avoid emitting duplicate events
	c.terminationTime = terminationTime

	// If termination time is in the past, don't emit event
	timeUntilTermination := int(math.Ceil(time.Until(terminationTime).Seconds()))
	if timeUntilTermination < 0 {
		return metadataAnswered
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
	return metadataAnswered
}

// checkRebalanceRecommendation checks for an AWS rebalance recommendation and emits an event if found.
// Skipped when a termination time is already known (termination supersedes the recommendation).
// Reports the outcome of its metadata lookup, if one was attempted.
func (c *Check) checkRebalanceRecommendation(sender sender.Sender) metadataOutcome {
	if c.cloudProvider == "" || c.preemptionUnsupported {
		return metadataNotAttempted
	}

	// If termination is already scheduled, a rebalance recommendation is redundant
	if !c.terminationTime.IsZero() {
		return metadataNotAttempted
	}

	// If rebalance recommendation was already emitted, don't emit again
	if !c.rebalanceNoticeTime.IsZero() {
		return metadataNotAttempted
	}

	// The rebalance lookup hits the same unreachable IMDS endpoint; back it
	// off on the same counter as the preemption lookup.
	if c.inMetadataBackoff() {
		log.Tracef("Skipping rebalance recommendation check, %d consecutive failures, backing off until %s", c.metadataFailures, c.metadataLastFailure.Add(metadataFailureBackoff))
		return metadataNotAttempted
	}

	noticeTime, err := getRebalanceRecommendationFn(context.Background(), c.cloudProvider)
	if err != nil {
		log.Tracef("Rebalance recommendation check returned an error (usually expected), cloud provider: %s, error: %s", c.cloudProvider, err)
		return metadataOutcomeFromError(err)
	}

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
	return metadataAnswered
}
