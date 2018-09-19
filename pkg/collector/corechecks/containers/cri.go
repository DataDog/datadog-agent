// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cri

package containers

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "gopkg.in/yaml.v2"
)

const (
	criCheckName = "cri"
)

// CRIConfig holds the config of the check
type CRIConfig struct {
	Tags        []string `yaml:"tags"`
	CollectDisk bool     `yaml:"collect_disk"`
}

// CRICheck grabs CRI metrics
type CRICheck struct {
	core.CheckBase
	instance *CRIConfig
}

func init() {
	core.RegisterCheck("cri", CRIFactory)
}

// CRIFactory is exported for integration testing
func CRIFactory() check.Check {
	return &CRICheck{
		CheckBase: core.NewCheckBase(criCheckName),
		instance:  &CRIConfig{},
	}
}

// Parse parses the CRICheck config and set default values
func (c *CRIConfig) Parse(data []byte) error {
	// default values
	c.CollectDisk = false

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (c *CRICheck) Configure(config, initConfig integration.Data) error {
	return c.instance.Parse(config)
}

// Run executes the check
func (c *CRICheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	util, err := cri.GetUtil()
	if err != nil {
		c.Warnf("Error initialising check: %s", err)
		return err
	}

	stats, err := util.ListContainerStats()
	if err != nil {
		c.Warnf("Cannot get containers from the CRI: %s", err)
		return err
	}
	for cid, stats := range stats {
		entityID := containers.BuildEntityName(util.Runtime, cid)
		tags, err := tagger.Tag(entityID, true)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", cid[:12], err)
		}
		tags = append(tags, "runtime:"+util.Runtime)
		tags = append(tags, c.instance.Tags...)
		sender.Gauge("cri.mem.rss", float64(stats.GetMemory().GetWorkingSetBytes().GetValue()), "", tags)
		// Cumulative CPU usage (sum across all cores) since object creation.
		sender.Rate("cri.cpu.usage", float64(stats.GetCpu().GetUsageCoreNanoSeconds().GetValue()), "", tags)
		if c.instance.CollectDisk {
			sender.Gauge("cri.disk.used", float64(stats.GetWritableLayer().GetUsedBytes().GetValue()), "", tags)
			sender.Gauge("cri.disk.inodes", float64(stats.GetWritableLayer().GetInodesUsed().GetValue()), "", tags)
		}
	}
	sender.Commit()
	return nil
}
