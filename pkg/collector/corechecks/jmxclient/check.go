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

// Check implements the JMX check using the JmxClient library
type Check struct {
	core.CheckBase
	instanceConfig *InstanceConfig
	initConfig     *InitConfig
	wrapper        *JmxClientWrapper
	sessionID      int
	lastRefresh    time.Time
	isConnected    bool
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase:   core.NewCheckBase(CheckName),
			sessionID:   -1,
			isConnected: false,
		}
	})
}

// Configure configures the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	// Call parent Configure
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return fmt.Errorf("common configure failed: %w", err)
	}

	// Parse instance config
	c.instanceConfig = &InstanceConfig{}
	if err := c.instanceConfig.Parse(config); err != nil {
		return fmt.Errorf("failed to parse instance config: %w", err)
	}

	// Parse init config
	c.initConfig = &InitConfig{}
	if err := c.initConfig.Parse(initConfig); err != nil {
		return fmt.Errorf("failed to parse init config: %w", err)
	}

	// Validate that we have bean collection configuration
	if len(c.initConfig.Conf) == 0 {
		return fmt.Errorf("no bean collection configuration provided in init_config.conf")
	}

	// Initialize the JmxClient wrapper
	c.wrapper = NewJmxClientWrapper()
	if c.wrapper == nil {
        return fmt.Errorf("can't create the JMXClient instance")
    }

	log.Infof("JMXClient check configured for instance: %s", c.instanceConfig.GetInstanceName())
	return nil
}

// Run executes the check
func (c *Check) Run() error {
	// Get sender
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender: %w", err)
	}

	log.Info("remy: before the connect call")

	// Ensure connection to JVM
	if !c.isConnected {
		if err := c.connect(); err != nil {
			c.Warnf("Failed to connect to JVM: %v", err)
			return err
		}
	}

	log.Info("remy: after the connect call")

	// Check if we need to refresh bean configuration
	if time.Since(c.lastRefresh) > time.Duration(c.instanceConfig.RefreshBeans)*time.Second {
		if err := c.refreshBeans(); err != nil {
			c.Warnf("Failed to refresh beans: %v", err)
			// Continue with collection even if refresh failed
		}
	}

    log.Info("remy: after the refresh beans call")

	// Collect metrics
	beans, err := c.wrapper.CollectBeansAsStructs(c.sessionID)
	if err != nil {
		c.Warnf("Failed to collect beans: %v", err)
		// Try to reconnect on next run
		c.isConnected = false
		return err
	}

	fmt.Println("remy:", beans)

	// Process and send metrics
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

	fmt.Println("remy.connect: before the connectJVM")

	// Connect using host:port if available
	if c.instanceConfig.Host != "" && c.instanceConfig.Port > 0 {
		sessionID, err = c.wrapper.ConnectJVM(c.instanceConfig.Host, c.instanceConfig.Port)
		if err != nil {
			return fmt.Errorf("failed to connect to JVM at %s:%d: %w",
				c.instanceConfig.Host, c.instanceConfig.Port, err)
		}
	} else {
		// TODO: Implement connection via JMX URL or process name regex
		return fmt.Errorf("only host:port connection is currently supported")
	}

	fmt.Println("remy.connect: after the connectJVM")

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

	// Marshal to JSON in the format expected by jmxclient
	configJSON, err := json.Marshal(beanRequests)
	if err != nil {
		return fmt.Errorf("failed to marshal bean config: %w", err)
	}

	log.Infof("Sending bean configuration to jmxclient: %s", string(configJSON))

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
	// Add instance tags
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
			metricName := fmt.Sprintf("jmx.%s.%s", bean.Path, attr.Name)

			// Try to parse the value as a number
			var numValue float64
			if _, err := fmt.Sscanf(attr.Value, "%f", &numValue); err == nil {
				sender.Gauge(metricName, numValue, "", tags)
			} else {
				log.Debugf("Skipping non-numeric metric %s with value: %s", metricName, attr.Value)
			}
		}
	}

	return nil
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
