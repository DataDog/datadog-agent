// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package jmxclient

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "jmxclient"
)

// MetricMetadata stores the alias and metric type for a collected metric
type MetricMetadata struct {
	Alias      string
	MetricType string // gauge, counter, rate, histogram, monotonic_count
}

// Check implements the JMX check using the JmxClient library
type Check struct {
	core.CheckBase
	instanceConfig *InstanceConfig
	initConfig     *InitConfig
	wrapper        *JmxClientWrapper
	sessionID      int
	lastRefresh    time.Time
	isConnected    bool

	// metricMetadata stores the mapping from "path:attribute" to metric metadata (alias and metric_type)
	metricMetadata map[string]MetricMetadata
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase:      core.NewCheckBase(CheckName),
			sessionID:      -1,
			isConnected:    false,
			metricMetadata: make(map[string]MetricMetadata),
		}
	})
}

// Configure configures the check
func (c *Check) Configure(senderManager sender.SenderManager, integrationDigest uint64, data, initConfig integration.Data, source string) error {
	// checks commons part
	c.instanceConfig = &InstanceConfig{}
	if err := c.instanceConfig.Parse(data); err != nil {
		return fmt.Errorf("failed to parse instance config: %w", err)
	}
	c.initConfig = &InitConfig{}
	if err := c.initConfig.Parse(initConfig); err != nil {
		return fmt.Errorf("failed to parse init config: %w", err)
	}

	// Validate that we have bean collection configuration
	// Either user-provided conf or CollectDefaultMetrics must be set
	if len(c.initConfig.Conf) == 0 && !c.initConfig.CollectDefaultMetrics {
		return fmt.Errorf("no bean collection configuration provided in init_config.conf and collect_default_metrics is not enabled")
	}
	c.BuildID(integrationDigest, data, initConfig)
	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	// jmxclient init

	wrapper, err := GetSharedWrapper()
	if err != nil {
		return fmt.Errorf("failed to get jmxclient wrapper: %w", err)
	}
	c.wrapper = wrapper
	log.Infof("jmxclient check configured for instance: %s", c.instanceConfig.GetInstanceName())
	return nil
}

// Run executes the check
func (c *Check) Run() error {
	// Get sender
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender: %w", err)
	}

	// are we connected?
	if !c.isConnected {
		if err := c.connect(); err != nil {
			c.Warnf("Failed to connect to JVM: %v", err)
			return err
		}
	}

	// regular refresh, in the case of some of the beans appearing later in time
	if time.Since(c.lastRefresh) > time.Duration(c.instanceConfig.RefreshBeans)*time.Second {
		if err := c.refreshBeans(); err != nil {
			c.Warnf("Failed to refresh: %v", err)
			// continue with collection even if refresh failed
		}
	}

	// collect data from beans
	beans, err := c.wrapper.CollectBeansAsStructs(c.sessionID)
	if err != nil {
		c.Warnf("Failed to collect: %v", err)
		// reconnect on next run
		c.isConnected = false
		return err
	}

	// process and send metrics
	if err := c.processMetrics(beans, sender); err != nil {
		c.Warnf("Failed to process metrics: %v", err)
		return err
	}

	// Commit metrics
	sender.Commit()

	return nil
}

// connect establishes a connection to the JVM
func (c *Check) connect() error {
	var sessionID int
	var err error

	// Connect using host:port if available
	if c.instanceConfig.Host != "" && c.instanceConfig.Port > 0 {
		sessionID, err = c.wrapper.ConnectJVM(c.instanceConfig.Host, c.instanceConfig.Port)
		if err != nil {
			return fmt.Errorf("failed to connect to JVM at %s:%d: %w",
				c.instanceConfig.Host, c.instanceConfig.Port, err)
		}
	} else {
		// TODO(remy): implement connection via JMX URL & process name
		return fmt.Errorf("only host:port connection is currently supported")
	}

	c.sessionID = sessionID
	c.isConnected = true

	log.Infof("Successfully connected to JVM, session ID: %d", sessionID)

	// Prepare beans after connection
	return c.refreshBeans()
}

// refreshBeans updates the bean collection configuration
func (c *Check) refreshBeans() error {
	// Convert bean configuration to jmxclient format
	beanRequests := ToJmxClientFormat(c.initConfig.Conf)

	// Build metric metadata mapping from bean requests
	c.metricMetadata = make(map[string]MetricMetadata)
	for _, req := range beanRequests {
		// Create key from path and attribute (with key if composite)
		key := fmt.Sprintf("%s:%s", req.Path, req.Attribute)
		if req.Key != "" {
			key = fmt.Sprintf("%s.%s", key, req.Key)
		}

		// Store metadata (alias and metric_type)
		metadata := MetricMetadata{
			Alias:      req.Alias,
			MetricType: req.MetricType,
		}
		// Default to gauge if no metric_type is specified
		if metadata.MetricType == "" {
			metadata.MetricType = "gauge"
		}
		c.metricMetadata[key] = metadata
	}

	// Marshal to JSON in the format expected by jmxclient
	configJSON, err := json.Marshal(beanRequests)
	if err != nil {
		return fmt.Errorf("failed to marshal bean config: %w", err)
	}

	log.Debugf("Sending bean configuration to jmxclient: %s", string(configJSON))

	// Send configuration to JmxClient
	if err := c.wrapper.PrepareBeans(c.sessionID, string(configJSON)); err != nil {
		return fmt.Errorf("failed to prepare beans: %w", err)
	}

	c.lastRefresh = time.Now()
	log.Debugf("Bean configuration refreshed for session %d", c.sessionID)

	return nil
}

// processMetrics processes collected metrics and sends them to the aggregator
func (c *Check) processMetrics(beans []BeanData, sender sender.Sender) error {
	// add instance tags
	tags := append([]string{}, c.instanceConfig.Tags...)
	tags = append(tags, fmt.Sprintf("jmx_instance:%s", c.instanceConfig.GetInstanceName()))

	// Process each bean
	for _, bean := range beans {
		if !bean.Success {
			log.Debugf("Skipping unsuccessful bean: %s", bean.Path)
			continue
		}

		// Process each attribute in the bean
		for _, attr := range bean.Attributes {
			// Try multiple keys to find the configured metric metadata
			// First try: path:attribute_name
			key := fmt.Sprintf("%s:%s", bean.Path, attr.Name)
			metadata, hasMetadata := c.metricMetadata[key]

			// Second try: if bean.Attribute is set, try path:bean.Attribute.attr.Name
			// (for composite attributes where bean.Attribute = "HeapMemoryUsage" and attr.Name = "used")
			if !hasMetadata && bean.Attribute != "" {
				compositeKey := fmt.Sprintf("%s:%s.%s", bean.Path, bean.Attribute, attr.Name)
				metadata, hasMetadata = c.metricMetadata[compositeKey]
			}

			// Determine metric name
			var metricName string
			if hasMetadata && metadata.Alias != "" {
				metricName = metadata.Alias
			} else {
				// Fallback to recomposed name if no alias is configured
				metricName = fmt.Sprintf("jmx.%s.%s", bean.Path, attr.Name)
			}

			// Determine metric type (default to gauge if not specified)
			metricType := "gauge"
			if hasMetadata && metadata.MetricType != "" {
				metricType = metadata.MetricType
			}

			// Try to parse the value as a number
			var numValue float64
			if _, err := fmt.Sscanf(attr.Value, "%f", &numValue); err == nil {
				// Send metric with the appropriate type
				c.sendMetric(sender, metricType, metricName, numValue, tags)
			} else {
				log.Debugf("Skipping non-numeric metric %s with value: %s", metricName, attr.Value)
			}
		}
	}

	return nil
}

// sendMetric sends a metric with the appropriate type to the aggregator
func (c *Check) sendMetric(sender sender.Sender, metricType string, name string, value float64, tags []string) {
	switch metricType {
	case "counter":
		// Counter: a value that can increase or decrease
		sender.Count(name, value, "", tags)
	case "monotonic_count":
		// Monotonic counter: a value that only increases
		sender.MonotonicCount(name, value, "", tags)
	case "rate":
		// Rate: a value representing a rate (e.g., requests per second)
		sender.Rate(name, value, "", tags)
	case "histogram":
		// Histogram: for statistical distribution
		sender.Histogram(name, value, "", tags)
	case "gauge":
		fallthrough
	default:
		// Gauge: snapshot value at a point in time (default)
		sender.Gauge(name, value, "", tags)
	}
}

// Stop stops the check
func (c *Check) Stop() {
	if c.isConnected && c.sessionID >= 0 {
		if err := c.wrapper.CloseJVM(c.sessionID); err != nil {
			log.Warnf("Failed to close JVM connection: %v", err)
		}
		c.isConnected = false
		c.sessionID = -1
	}
}

// Cancel cancels the check
func (c *Check) Cancel() {
	c.Stop()
}

// Interval returns the scheduling interval for the check
func (c *Check) Interval() time.Duration {
	// Default to 15 seconds if not specified
	// This can be made configurable via instance config
	return 15 * time.Second
}
