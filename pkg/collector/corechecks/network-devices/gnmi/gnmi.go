// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmi implements the gNMI core check for network device monitoring.
package gnmi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/report"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check.
	CheckName = "gnmi"
)

// Check collects gNMI telemetry and reports snmp.* metrics.
type Check struct {
	core.CheckBase

	mu       sync.Mutex
	config   *config.CheckConfig
	interval time.Duration
	client   *client.Client
	started  bool
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		interval:  time.Duration(config.DefaultMinCollectionInterval) * time.Second,
	}
}

// Configure parses instance configuration and prepares the gNMI client.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	if !config.IsEnabled() {
		return errors.New("gNMI core check is disabled; set network_devices.gnmi.enabled to true to enable it")
	}

	checkConfig, err := config.NewCheckConfig(rawInstance)
	if err != nil {
		return fmt.Errorf("build config failed: %w", err)
	}
	log.Debugf("gNMI configuration: %s", checkConfig.String())

	instanceName := fmt.Sprintf("%s:%d", checkConfig.Instance.Address, checkConfig.Instance.Port)
	if setNameErr := rawInstance.SetNameForInstance(instanceName); setNameErr != nil {
		log.Warnf("error setting check name (instance=%s): %s", instanceName, setNameErr)
	}

	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	if err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider); err != nil {
		return fmt.Errorf("common configure failed: %w", err)
	}

	gnmiClient, err := client.New(client.Config{
		Address:  checkConfig.Instance.Address,
		Port:     checkConfig.Instance.Port,
		Username: checkConfig.Instance.Username,
		Password: checkConfig.Instance.Password,
		Profile:  checkConfig.Profile,
	})
	if err != nil {
		return fmt.Errorf("create gNMI client failed: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = checkConfig
	c.interval = time.Duration(checkConfig.Instance.MinCollectionInterval) * time.Second
	c.client = gnmiClient
	c.started = false

	return nil
}

// Run snapshots the client cache and submits metrics.
func (c *Check) Run() error {
	if err := c.ensureClientStarted(); err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}

	c.mu.Lock()
	gnmiClient := c.client
	checkConfig := c.config
	c.mu.Unlock()

	if gnmiClient == nil || checkConfig == nil {
		return errors.New("gNMI check is not configured")
	}

	now := time.Now()
	snapshot := gnmiClient.Snapshot()
	stalenessThreshold := report.DefaultStalenessThreshold(c.interval)
	freshSnapshot := report.FilterStale(snapshot, stalenessThreshold, now)

	healthStats := report.HealthStats{
		StreamState:      gnmiClient.StreamState(),
		ReconnectCount:   gnmiClient.ReconnectAttempts(),
		ReceivedSamples:  gnmiClient.ReceivedSamples(),
		SampleAgeSeconds: report.OldestSampleAgeSeconds(snapshot, now),
	}
	if err := report.ReportHealth(s, checkConfig, healthStats); err != nil {
		return err
	}

	if len(freshSnapshot) > 0 {
		if err := report.ReportMetrics(s, checkConfig, freshSnapshot); err != nil {
			return err
		}
	}

	s.Commit()
	return nil
}

// Cancel stops the gNMI client and waits for background goroutines to exit.
func (c *Check) Cancel() {
	c.mu.Lock()
	gnmiClient := c.client
	c.client = nil
	c.config = nil
	c.started = false
	c.mu.Unlock()

	if gnmiClient != nil {
		if err := gnmiClient.Close(); err != nil {
			log.Warnf("error closing gNMI client: %s", err)
		}
	}
}

// Interval returns the scheduling time for the check.
func (c *Check) Interval() time.Duration {
	return c.interval
}

// IsHASupported returns true if the check supports HA.
func (c *Check) IsHASupported() bool {
	return true
}

// String returns a redacted representation safe for logs.
func (c *Check) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config == nil {
		return CheckName
	}
	return c.config.String()
}

func (c *Check) ensureClientStarted() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}
	if c.client == nil {
		return errors.New("gNMI check is not configured")
	}

	if err := c.client.Start(context.Background()); err != nil {
		return fmt.Errorf("start gNMI client failed: %w", err)
	}

	c.started = true
	return nil
}
