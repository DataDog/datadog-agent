// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package middleware

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// CheckWrapper cleans up the check sender after a check was
// descheduled, taking care to postpone it until Run returns if it is
// running.
type CheckWrapper struct {
	inner check.Check
	wg    sync.WaitGroup
}

// NewCheckWrapper returns a wrapped check.
func NewCheckWrapper(inner check.Check) *CheckWrapper {
	return &CheckWrapper{
		inner: inner,
	}
}

// Run implements Check#Run
func (c *CheckWrapper) Run() error {
	c.wg.Add(1)
	defer c.wg.Done()
	return c.inner.Run()
}

// Cancel implements Check#Cancel
func (c *CheckWrapper) Cancel() {
	c.inner.Cancel()
	go c.destroySender()
}

func (c *CheckWrapper) destroySender() {
	c.wg.Wait()
	aggregator.DestroySender(c.ID())
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
func (c *CheckWrapper) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	return c.inner.Configure(integrationConfigDigest, config, initConfig, source)
}

// Interval implements Check#Interval
func (c *CheckWrapper) Interval() time.Duration {
	return c.inner.Interval()
}

// ID implements Check#ID
func (c *CheckWrapper) ID() check.ID {
	return c.inner.ID()
}

// GetWarnings implements Check#GetWarnings
func (c *CheckWrapper) GetWarnings() []error {
	return c.inner.GetWarnings()
}

// GetSenderStats implements Check#GetSenderStats
func (c *CheckWrapper) GetSenderStats() (check.SenderStats, error) {
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
