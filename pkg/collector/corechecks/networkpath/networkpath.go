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
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
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

	cfg := traceroute.Config{
		DestHostname: c.config.DestHostname,
		DestPort:     c.config.DestPort,
		MaxTTL:       c.config.MaxTTL,
		TimeoutMs:    c.config.TimeoutMs,
	}

	tr := traceroute.New(cfg)
	path, err := tr.Run()
	if err != nil {
		return fmt.Errorf("failed to trace path: %w", err)
	}

	// Add tags to path
	commonTags := c.getCommonTags()
	path.Tags = commonTags

	// send to EP
	err = c.SendNetPathMDToEP(senderInstance, path)
	if err != nil {
		return fmt.Errorf("failed to send network path metadata: %w", err)
	}

	metricTags := c.getCommonTagsForMetrics()
	metricTags = append(metricTags, commonTags...)
	c.submitTelemetryMetrics(senderInstance, path, startTime, metricTags)

	senderInstance.Commit()
	return nil
}

func (c *Check) getCommonTags() []string {
	tags := utils.CopyStrings(c.config.Tags)

	agentHost, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting the hostname: %v", err)
	} else {
		tags = append(tags, "agent_host:"+agentHost)
	}

	tags = append(tags, utils.GetAgentVersionTag())

	return tags
}

func (c *Check) getCommonTagsForMetrics() []string {
	destPortTag := "unspecified"
	if c.config.DestPort > 0 {
		destPortTag = strconv.Itoa(int(c.config.DestPort))
	}
	tags := []string{
		"protocol:udp", // TODO: Update to protocol from config when we support tcp/icmp
		"destination_hostname:" + c.config.DestHostname,
		"destination_port:" + destPortTag,
	}
	return tags
}

// SendNetPathMDToEP sends a traced network path to EP
func (c *Check) SendNetPathMDToEP(sender sender.Sender, path traceroute.NetworkPath) error {
	payloadBytes, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %s", err)
	}
	log.Debugf("traceroute path metadata payload: %s", string(payloadBytes))
	sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkPath)
	return nil
}

func (c *Check) submitTelemetryMetrics(senderInstance sender.Sender, path traceroute.NetworkPath, startTime time.Time, tags []string) {
	newTags := utils.CopyStrings(tags)

	checkDuration := time.Since(startTime)
	senderInstance.Gauge("datadog.network_path.check_duration", checkDuration.Seconds(), "", newTags)

	if !c.lastCheckTime.IsZero() {
		checkInterval := startTime.Sub(c.lastCheckTime)
		senderInstance.Gauge("datadog.network_path.check_interval", checkInterval.Seconds(), "", newTags)
	}
	c.lastCheckTime = startTime

	senderInstance.Gauge("datadog.network_path.path.monitored", float64(1), "", newTags)
	if len(path.Hops) > 0 {
		lastHop := path.Hops[len(path.Hops)-1]
		if lastHop.Success {
			senderInstance.Gauge("datadog.network_path.path.hops", float64(len(path.Hops)), "", newTags)
		}
		senderInstance.Gauge("datadog.network_path.path.reachable", float64(utils.BoolToFloat64(lastHop.Success)), "", newTags)
		senderInstance.Gauge("datadog.network_path.path.unreachable", float64(utils.BoolToFloat64(!lastHop.Success)), "", newTags)
	}
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.MinCollectionInterval
}

// Configure the networkpath check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source)
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
