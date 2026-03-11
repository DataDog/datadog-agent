// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package test_check implements the test_check check.
package test_check

import (
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check.
const CheckName = "test_check"

// Configuration holds the instance-level configuration for TestCheckCheck.
// Tags, service, and min_collection_interval are handled by CommonConfigure via CheckBase.
type Configuration struct {
	RequiredParam string `yaml:"required_param"`
	FakeParam     string `yaml:"fake_param"`
}

// TestCheckCheck implements the check.Check interface.
type TestCheckCheck struct {
	core.CheckBase
	config Configuration
}

// Factory returns a new instance of TestCheckCheck.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &TestCheckCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure parses the check configuration.
func (c *TestCheckCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest_ uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}
	return yaml.Unmarshal(data, &c.config)
}

// Run executes the check.
func (c *TestCheckCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// TODO: implement check logic here
	// sender.Gauge("metric.name", value, "", nil)

	sender.Commit()
	return nil
}
