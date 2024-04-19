// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check.
	CheckName              = "service_discovery"
	defaultRefreshInterval = 60
)

// Config holds the check configuration.
type config struct {
	RefreshIntervalSeconds int `yaml:"refresh_interval_seconds"`
}

// Parse parses the configuration
func (c *config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.RefreshIntervalSeconds <= 0 {
		c.RefreshIntervalSeconds = defaultRefreshInterval
	}
	return nil
}

// Check ...
type Check struct {
	corechecks.CheckBase
	stopCh chan struct{}
	cfg    *config
}

// Factory returns a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return corechecks.NewLongRunningCheckWrapper(&Check{
			CheckBase: corechecks.NewCheckBase(CheckName),
			stopCh:    make(chan struct{}),
		})
	})
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, cfgRaw, initConfig integration.Data, source string) error {
	if !agentconfig.Datadog.GetBool("service_discovery.enabled") {
		// TODO: ignore for now
		// return errors.New("service discovery is disabled")
	}
	if err := c.cfg.Parse(cfgRaw); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	return nil
}

// Run starts the container_image check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	ticker := time.NewTicker(time.Duration(c.cfg.RefreshIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		c.discoverServices()

		select {
		case <-c.stopCh:
			return nil

		case <-ticker.C:
			continue
		}
	}
}

// Stop stops the check.
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0. It makes it a long-running check.
func (c *Check) Interval() time.Duration {
	return 0
}
