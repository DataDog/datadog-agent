// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package middleware contains a check wrapper helper
package middleware

import (
	"context"
	"errors"
	"sync"
	"time"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
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

	inner check.Check
	// done is true when the check was cancelled and must not run.
	done bool
	// Locked while check is running.
	runM sync.Mutex

	// Remote-query admission never takes runM: scheduled and remote runs overlap.
	remoteM       sync.Mutex
	remoteStopped bool
	remoteCalls   sync.WaitGroup
}

// NewCheckWrapper returns a wrapped check.
func NewCheckWrapper(inner check.Check, senderManager sender.SenderManager, agentTelemetry option.Option[agenttelemetry.Component], issueReporter option.Option[healthplatformstore.Component]) *CheckWrapper {
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
	return c.inner.Run()
}

// Cancel implements Check#Cancel
func (c *CheckWrapper) Cancel() {
	c.StopRemoteQueries()
	c.inner.Cancel()
	// The integration's cancel waits for its runs. Also keep the wrapper alive
	// until every admitted bridge call (including cancellation) has returned.
	c.remoteCalls.Wait()
	go c.destroySender()
}

// StopRemoteQueries closes admission before the check is signaled or torn down.
// It does not wait for any run and is safe to call while the collector is locked.
func (c *CheckWrapper) StopRemoteQueries() {
	c.remoteM.Lock()
	c.remoteStopped = true
	c.remoteM.Unlock()
}

func (c *CheckWrapper) admitRemoteQuery() (check.RemoteQuerier, error) {
	c.remoteM.Lock()
	defer c.remoteM.Unlock()
	if c.remoteStopped {
		return nil, check.ErrRemoteQueryCancelled
	}
	inner, ok := c.inner.(check.RemoteQuerier)
	if !ok {
		return nil, errors.New("remote queries are not supported")
	}
	c.remoteCalls.Add(1)
	return inner, nil
}

// RemoteQuery delegates a concurrently admitted call without locking scheduled Run.
func (c *CheckWrapper) RemoteQuery(ctx context.Context, request string) (string, error) {
	inner, err := c.admitRemoteQuery()
	if err != nil {
		return "", err
	}
	defer c.remoteCalls.Done()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return inner.RemoteQuery(ctx, request)
}

// CancelRemoteQuery cancels a run only while this check still admits calls.
func (c *CheckWrapper) CancelRemoteQuery(crawlID string) error {
	inner, err := c.admitRemoteQuery()
	if err != nil {
		return err
	}
	defer c.remoteCalls.Done()
	return inner.CancelRemoteQuery(crawlID)
}

// SupportsRemoteQueries reports whether the underlying check exposes remote queries.
func (c *CheckWrapper) SupportsRemoteQueries() bool {
	inner, ok := c.inner.(check.RemoteQuerier)
	return ok && inner.SupportsRemoteQueries()
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
