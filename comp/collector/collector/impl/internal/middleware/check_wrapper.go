// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package middleware contains a check wrapper helper
package middleware

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"sync"
	"time"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/checkexecfailure"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckWrapper cleans up the check sender after a check was
// descheduled, taking care that Run is not executing during or after
// that.
type CheckWrapper struct {
	senderManager  sender.SenderManager
	agentTelemetry option.Option[agenttelemetry.Component]
	healthPlatform option.Option[healthplatformstore.Component]
	config         config.Component

	inner check.Check
	// done is true when the check was cancelled and must not run.
	done bool
	// Locked while check is running.
	runM sync.Mutex

	// consecutiveFailures and consecutiveSuccesses are only touched from Run,
	// which runM serializes, so they need no separate lock.
	consecutiveFailures  int
	consecutiveSuccesses int
	execFailureReported  bool
}

// NewCheckWrapper returns a wrapped check.
func NewCheckWrapper(inner check.Check, senderManager sender.SenderManager, agentTelemetry option.Option[agenttelemetry.Component], issueReporter option.Option[healthplatformstore.Component], cfg config.Component) *CheckWrapper {
	if reporter, isSet := issueReporter.Get(); isSet {
		if aware, ok := check.As[check.IssueAwareCheck](inner); ok {
			aware.SetIssueReporter(reporter)
		}
	}
	if override, ok := check.SenderManagerOverride(inner); ok {
		senderManager = override
	}
	return &CheckWrapper{
		inner:          inner,
		senderManager:  senderManager,
		agentTelemetry: agentTelemetry,
		healthPlatform: issueReporter,
		config:         cfg,
	}
}

// Run implements Check#Run
func (c *CheckWrapper) Run() (err error) {
	c.runM.Lock()
	defer c.runM.Unlock()
	if c.done {
		return nil
	}

	// Start telemetry span if telemetry is enabled
	if telemetry, isSet := c.agentTelemetry.Get(); isSet {
		span, _ := telemetry.StartStartupSpan("check." + c.inner.String())
		defer span.Finish(err)
	}

	// Run the check
	err = c.inner.Run()
	c.trackExecutionResult(err)
	return err
}

// trackExecutionResult updates the consecutive failure/success counters for
// this check and reports or resolves a check-execution-failure health issue
// once the configured thresholds are crossed. No-op when the health platform
// is unavailable or the feature is disabled.
//
// Because the issue id is scoped by IssueDiscriminator (see
// comp/healthplatform/README.md, "Cluster-wide issue collapse") rather than
// hostname, the first agent in the DaemonSet to recover resolves the issue
// for every other node still affected — correct for a shared fix, but it can
// flap if only some agents recover.
func (c *CheckWrapper) trackExecutionResult(runErr error) {
	hp, ok := c.healthPlatform.Get()
	if !ok || !c.config.GetBool("health_platform.check_execution_failure.enabled") {
		return
	}

	if runErr != nil {
		c.consecutiveSuccesses = 0
		c.consecutiveFailures++
		if c.consecutiveFailures >= c.config.GetInt("health_platform.check_execution_failure.consecutive_failures") {
			c.reportExecutionFailure(hp, runErr)
		}
		return
	}

	c.consecutiveFailures = 0
	c.consecutiveSuccesses++
	if c.execFailureReported && c.consecutiveSuccesses >= c.config.GetInt("health_platform.check_execution_failure.consecutive_successes") {
		hp.ResolveIssue(executionFailureIssueID(hp, c.ID()))
		c.execFailureReported = false
	}
}

// executionFailureIssueID derives the health-issue id for a check-execution
// failure on the given check id, scoped to hp's issue discriminator (the
// agent's DaemonSet uid when resolvable, so identical failures across a
// cluster collapse into one issue).
func executionFailureIssueID(hp healthplatformstore.Component, checkID checkid.ID) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", hp.IssueDiscriminator(""), checkID)
	return fmt.Sprintf("%s:%016x", checkexecfailure.IssueID, h.Sum64())
}

// reportExecutionFailure reports a health-platform issue for a check that
// has failed to run for the configured number of consecutive intervals.
func (c *CheckWrapper) reportExecutionFailure(hp healthplatformstore.Component, runErr error) {
	issue, err := checkexecfailure.NewCheckExecFailureIssue().BuildIssue(map[string]string{
		"check_name":           c.inner.String(),
		"errors":               runErr.Error(),
		"consecutive_failures": strconv.Itoa(c.consecutiveFailures),
	})
	if err != nil {
		return
	}
	issue.Id = executionFailureIssueID(hp, c.ID())
	if err := hp.ReportIssue(issue); err == nil {
		c.execFailureReported = true
	}
}

// Cancel implements Check#Cancel
func (c *CheckWrapper) Cancel() {
	c.inner.Cancel()
	go c.destroySender()
}

func (c *CheckWrapper) destroySender() {
	// Done must happen before Wait
	c.runM.Lock()
	defer c.runM.Unlock()
	c.done = true
	c.senderManager.DestroySender(c.ID())
}

// Stop implements Check#Stop
func (c *CheckWrapper) Stop() {
	c.inner.Stop()
}

// String implements Check#String
func (c *CheckWrapper) String() string {
	return c.inner.String()
}

// Configure implements Check#Configure
func (c *CheckWrapper) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	if c.senderManager == nil {
		c.senderManager = senderManager
	}
	return c.inner.Configure(c.senderManager, integrationConfigDigest, config, initConfig, source, provider)
}

// Interval implements Check#Interval
func (c *CheckWrapper) Interval() time.Duration {
	return c.inner.Interval()
}

// ID implements Check#ID
func (c *CheckWrapper) ID() checkid.ID {
	return c.inner.ID()
}

// Unwrap returns the wrapped check.
func (c *CheckWrapper) Unwrap() check.Check {
	return c.inner
}

// GetWarnings implements Check#GetWarnings
func (c *CheckWrapper) GetWarnings() []error {
	return c.inner.GetWarnings()
}

// GetSenderStats implements Check#GetSenderStats
func (c *CheckWrapper) GetSenderStats() (stats.SenderStats, error) {
	return c.inner.GetSenderStats()
}

// Version implements Check#Version
func (c *CheckWrapper) Version() string {
	return c.inner.Version()
}

// ConfigSource implements Check#ConfigSource
func (c *CheckWrapper) ConfigSource() string {
	return c.inner.ConfigSource()
}

// ConfigProvider implements Check#ConfigProvider
func (c *CheckWrapper) ConfigProvider() string {
	return c.inner.ConfigProvider()
}

// Loader returns the name of the check loader
func (c *CheckWrapper) Loader() string {
	return c.inner.Loader()
}

// IsTelemetryEnabled implements Check#IsTelemetryEnabled
func (c *CheckWrapper) IsTelemetryEnabled() bool {
	return c.inner.IsTelemetryEnabled()
}

// InitConfig implements Check#InitConfig
func (c *CheckWrapper) InitConfig() string {
	return c.inner.InitConfig()
}

// InstanceConfig implements Check#InstanceConfig
func (c *CheckWrapper) InstanceConfig() string {
	return c.inner.InstanceConfig()
}

// GetDiagnoses returns the diagnoses cached in last run or diagnose explicitly
func (c *CheckWrapper) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	// Avoid running concurrently with Run method (for now)
	c.runM.Lock()
	defer c.runM.Unlock()

	if c.done {
		return nil, nil
	}
	return c.inner.GetDiagnoses()
}

// IsHASupported implements Check#IsHASupported
func (c *CheckWrapper) IsHASupported() bool {
	return c.inner.IsHASupported()
}
