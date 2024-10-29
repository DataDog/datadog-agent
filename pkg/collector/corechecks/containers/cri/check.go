// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

// Package cri implements the cri check.
package cri

import (
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check// CheckName
	CheckName     = "cri"
	cacheValidity = 2 * time.Second
)

// CRIConfig holds the config of the check
type CRIConfig struct {
	CollectDisk bool `yaml:"collect_disk"`
}

// CRICheck grabs CRI metrics
type CRICheck struct {
	core.CheckBase
	instance  *CRIConfig
	processor generic.Processor
	store     workloadmeta.Component
	tagger    tagger.Component
}

// Factory is exported for integration testing
func Factory(store workloadmeta.Component, tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return &CRICheck{
			CheckBase: core.NewCheckBase(CheckName),
			instance:  &CRIConfig{},
			store:     store,
			tagger:    tagger,
		}
	})
}

// Parse parses the CRICheck config and set default values
func (c *CRIConfig) Parse(data []byte) error {
	// default values
	c.CollectDisk = false

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (c *CRICheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	var err error
	if err = c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err = c.instance.Parse(config); err != nil {
		return err
	}

	containerFilter, err := containers.GetSharedMetricFilter()
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
	}

	c.processor = generic.NewProcessor(metrics.GetProvider(optional.NewOption(c.store)), generic.NewMetadataContainerAccessor(c.store), metricsAdapter{}, getProcessorFilter(containerFilter, c.store), c.tagger)
	if c.instance.CollectDisk {
		c.processor.RegisterExtension("cri-custom-metrics", &criCustomMetricsExtension{criGetter: func() (cri.CRIClient, error) {
			return cri.GetUtil()
		}})
	}

	return nil
}

// Run executes the check
func (c *CRICheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	return c.runProcessor(sender)
}

func (c *CRICheck) runProcessor(sender sender.Sender) error {
	return c.processor.Run(sender, cacheValidity)
}

func getProcessorFilter(legacyFilter *containers.Filter, store workloadmeta.Component) generic.ContainerFilter {
	// Reject all containers that are not run by Docker
	return generic.ANDContainerFilter{
		Filters: []generic.ContainerFilter{
			generic.FuncContainerFilter(func(container *workloadmeta.Container) bool {
				return container.Labels[kubernetes.CriContainerNamespaceLabel] == ""
			}),
			generic.LegacyContainerFilter{OldFilter: legacyFilter, Store: store},
		},
	}
}
