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
	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/telemetry"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName defines the name of the
// Network Path check
const CheckName = "network_path"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config        *CheckConfig
	lastCheckTime time.Time
	traceroute    traceroute.Component
	telemetryComp telemetryComp.Component
}

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()
	senderInstance, err := c.GetSender()
	if err != nil {
		return err
	}
	metricSender := metricsender.NewMetricSenderAgent(senderInstance)

	cfg := config.Config{
		DestHostname:              c.config.DestHostname,
		DestPort:                  c.config.DestPort,
		MaxTTL:                    c.config.MaxTTL,
		Timeout:                   c.config.Timeout,
		Protocol:                  c.config.Protocol,
		TCPMethod:                 c.config.TCPMethod,
		TCPSynParisTracerouteMode: c.config.TCPSynParisTracerouteMode,
		DisableWindowsDriver:      c.config.DisableWindowsDriver,
		ReverseDNS:                true,
		TracerouteQueries:         c.config.TracerouteQueries,
		E2eQueries:                c.config.E2eQueries,
	}

	path, err := c.traceroute.Run(context.TODO(), cfg)
	if err != nil {
		return fmt.Errorf("failed to trace path: %w", err)
	}

	err = payload.ValidateNetworkPath(&path)
	if err != nil {
		return fmt.Errorf("failed to validate network path: %w", err)
	}

	path.Namespace = c.config.Namespace
	path.Origin = payload.PathOriginNetworkPathIntegration
	path.TestRunType = payload.TestRunTypeScheduled
	path.SourceProduct = payload.GetSourceProduct(pkgconfigsetup.Datadog().GetString("infrastructure_mode"))
	path.CollectorType = payload.CollectorTypeAgent

	// Add tags to path
	path.Source.Service = c.config.SourceService
	path.Destination.Service = c.config.DestinationService
	path.Tags = append(path.Tags, c.config.Tags...)

	// send to EP
	err = c.SendNetPathMDToEP(senderInstance, path)
	if err != nil {
		return fmt.Errorf("failed to send network path metadata: %w", err)
	}

	metricTags := append(utils.GetCommonAgentTags(), c.config.Tags...)

	// TODO: Remove static path telemetry code (separate PR)
	c.submitTelemetry(metricSender, path, metricTags, startTime)

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

	telemetry.SubmitNetworkPathTelemetry(metricSender, path, checkDuration, checkInterval, metricTags)
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

// IsHASupported returns true if the check supports HA
func (c *Check) IsHASupported() bool {
	return true
}

// Factory creates a new check factory
func Factory(telemetry telemetryComp.Component, traceroute traceroute.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase:     core.NewCheckBase(CheckName),
			telemetryComp: telemetry,
			traceroute:    traceroute,
		}
	})
}
