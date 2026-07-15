// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"errors"
	"fmt"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Check is the OpenMetrics core check.
type Check struct {
	core.CheckBase
	scraper *Scraper
}

// Run executes one OpenMetrics scrape.
func (c *Check) Run() error {
	if c.scraper == nil {
		return errors.New("openmetrics check is not configured")
	}
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	return c.scraper.Scrape(sender)
}

// Configure parses the OpenMetrics instance configuration.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	if !pkgconfigsetup.Datadog().GetBool("openmetrics.use_core_loader") {
		return fmt.Errorf("%w: openmetrics core check is disabled", check.ErrSkipCheckInstance)
	}

	c.BuildID(integrationConfigDigest, data, initConfig)

	cfg, err := parseConfigWithInit(data, initConfig)
	if err != nil {
		if errors.Is(err, errUnsupportedCoreConfig) {
			recordConfigureTelemetry(configureOutcomeFallback, unsupportedConfigTelemetryReason(err))
			return fmt.Errorf("%w: %v", check.ErrSkipCheckInstance, err)
		}
		recordConfigureTelemetry(configureOutcomeError, configureReasonParseConfig)
		return err
	}

	commonInstance := stripOpenMetricsHandledCommonConfig(data)
	if err := c.CommonConfigure(senderManager, initConfig, commonInstance, source, provider); err != nil {
		return err
	}
	cfg.checkID = string(c.ID())
	scraper, err := newScraper(cfg)
	if err != nil {
		if errors.Is(err, errUnsupportedCoreConfig) {
			recordConfigureTelemetry(configureOutcomeFallback, unsupportedConfigTelemetryReason(err))
			return fmt.Errorf("%w: %v", check.ErrSkipCheckInstance, err)
		}
		recordConfigureTelemetry(configureOutcomeError, configureReasonNewScraper)
		return err
	}
	c.scraper = &Scraper{inner: scraper}
	s, err := c.GetSender()
	if err != nil {
		recordConfigureTelemetry(configureOutcomeError, configureReasonNewScraper)
		return err
	}
	s.FinalizeCheckServiceTag()
	recordConfigureTelemetry(configureOutcomeLoaded, configureReasonNone)
	return nil
}

func stripOpenMetricsHandledCommonConfig(raw integration.Data) integration.Data {
	if len(raw) == 0 {
		return raw
	}

	var config map[interface{}]interface{}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return raw
	}
	delete(config, "tags")

	encoded, err := yaml.Marshal(config)
	if err != nil {
		return raw
	}
	return integration.Data(encoded)
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{CheckBase: core.NewCheckBase(CheckName)}
}
