// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package networkpath defines the agent corecheck for
// the Network Path integration
package networkpath

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
)

// CheckName defines the name of the
// Network Path check
const CheckName = "network_path"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config        *CheckConfig
	lastCheckTime time.Time
}

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()
	senderInstance, err := c.GetSender()
	if err != nil {
		return err
	}
	metricSender := metricsender.NewMetricSenderAgent(senderInstance)

	cfg := traceroute.Config{
		DestHostname: c.config.DestHostname,
		DestPort:     c.config.DestPort,
		MaxTTL:       c.config.MaxTTL,
		TimeoutMs:    c.config.TimeoutMs,
	}

	tr, err := traceroute.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize traceroute: %w", err)
	}
	path, err := tr.Run(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to trace path: %w", err)
	}
	path.Namespace = c.config.Namespace

	// Add tags to path
	commonTags := append(utils.GetCommonAgentTags(), c.config.Tags...)
	path.Source.Service = c.config.SourceService
	path.Destination.Service = c.config.DestinationService
	path.Tags = commonTags

	// send to EP
	err = c.SendNetPathMDToEP(senderInstance, path)
	if err != nil {
		return fmt.Errorf("failed to send network path metadata: %w", err)
	}

	c.submitTelemetry(metricSender, path, commonTags, startTime)

	senderInstance.Commit()
	return nil
}

// SendNetPathMDToEP sends a traced network path to EP
func (c *Check) SendNetPathMDToEP(sender sender.Sender, path payload.NetworkPath) error {
	payloadBytes, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %s", err)
	}
	log.Debugf("traceroute path metadata payload: %s", string(payloadBytes))
	sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkPath)
	return nil
}

func (c *Check) submitTelemetry(metricSender metricsender.MetricSender, path payload.NetworkPath, metricTags []string, startTime time.Time) {
	var checkInterval time.Duration
	if !c.lastCheckTime.IsZero() {
		checkInterval = startTime.Sub(c.lastCheckTime)
	}
	c.lastCheckTime = startTime
	checkDuration := time.Since(startTime)

	telemetry.SubmitNetworkPathTelemetry(metricSender, path, telemetry.CollectorTypeNetworkPathIntegration, checkDuration, checkInterval, metricTags)
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.MinCollectionInterval
}

// Configure the networkpath check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	config, err := NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return err
	}
	c.config = config
	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
