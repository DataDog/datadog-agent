// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package generic implements the container check.
package generic

import (
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName     = "container"
	cacheValidity = 2 * time.Second
)

// ContainerConfig holds the check configuration
type ContainerConfig struct{}

// Parse parses the container check config and set default values
func (c *ContainerConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// ContainerCheck generates metrics for all containers
type ContainerCheck struct {
	core.CheckBase
	instance  *ContainerConfig
	processor Processor
	store     workloadmeta.Component
	tagger    tagger.Component
}

// Factory returns a new check factory
func Factory(store workloadmeta.Component, tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return &ContainerCheck{
			CheckBase: core.NewCheckBase(CheckName),
			instance:  &ContainerConfig{},
			store:     store,
			tagger:    tagger,
		}
	})
}

// Configure parses the check configuration and init the check
func (c *ContainerCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	filter, err := containers.GetSharedMetricFilter()
	if err != nil {
		return err
	}
	c.processor = NewProcessor(metrics.GetProvider(optional.NewOption(c.store)), NewMetadataContainerAccessor(c.store), GenericMetricsAdapter{}, LegacyContainerFilter{OldFilter: filter, Store: c.store}, c.tagger)
	return c.instance.Parse(config)
}

// Run executes the check
func (c *ContainerCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	return c.processor.Run(sender, cacheValidity)
}
