// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package openmetrics implements a Go core check that scrapes Prometheus /
// OpenMetrics endpoints and forwards the collected metrics to Datadog.
package openmetrics

import (
	"fmt"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/openmetrics/scraper"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check as registered with the collector.
	CheckName = "openmetrics"
)

// Check collects metrics from a Prometheus / OpenMetrics endpoint.
type Check struct {
	core.CheckBase
	scraper *scraper.Scraper
}

// Factory returns a factory function that creates new OpenMetrics check instances.
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
		}
	})
}

// Configure parses the check instance configuration and initialises the scraper.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64,
	data integration.Data, initConfig integration.Data, source string, provider string) error {

	cfg, err := scraper.ParseConfig([]byte(data))
	if err != nil {
		return fmt.Errorf("openmetrics: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("openmetrics: %w", err)
	}

	// Apply init_config defaults (currently a placeholder for future use).
	var initCfg initConfiguration
	if err := yaml.Unmarshal(initConfig, &initCfg); err != nil {
		log.Debugf("openmetrics: could not parse init_config: %v", err)
	}

	s, err := scraper.NewScraper(cfg)
	if err != nil {
		return fmt.Errorf("openmetrics: failed to create scraper: %w", err)
	}
	c.scraper = s

	// BuildID so multiple instances of the same check get unique IDs.
	c.BuildID(integrationConfigDigest, data, initConfig)

	return c.CommonConfigure(senderManager, initConfig, data, source, provider)
}

// Run executes a single collection cycle.
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return err
	}
	defer snd.Commit()

	return c.scraper.Scrape(snd)
}

// initConfiguration holds init_config level settings.
type initConfiguration struct {
	// Placeholder — the openmetrics check currently has no init_config options.
}
