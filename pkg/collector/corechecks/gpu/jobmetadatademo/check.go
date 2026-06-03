// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package jobmetadatademo implements a small demo check for GPU job metadata enrichment.
package jobmetadatademo

import (
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the demo check.
	CheckName = "gpu_job_metadata_demo"

	defaultContainerID           = "demo-container"
	defaultMinCollectionInterval = 5 * time.Second
	metricName                   = "gpu.job_metadata_demo.value"
)

type instanceConfig struct {
	ContainerID string `yaml:"container_id"`
	LogResults  bool   `yaml:"log_results"`
}

// Check emits a placeholder metric tagged with current tagger tags for one container.
type Check struct {
	core.CheckBase
	tagger      taggerdef.Component
	containerID string
	logResults  bool
}

// Factory creates a new check factory.
func Factory(taggerComponent taggerdef.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check { return newCheck(taggerComponent) })
}

func newCheck(tagger taggerdef.Component) check.Check {
	return &Check{
		CheckBase:   core.NewCheckBaseWithInterval(CheckName, defaultMinCollectionInterval),
		tagger:      tagger,
		containerID: defaultContainerID,
		logResults:  true,
	}
}

// Configure handles initial configuration for the demo check.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	conf := instanceConfig{
		ContainerID: defaultContainerID,
		LogResults:  true,
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}

	conf.ContainerID = strings.TrimSpace(conf.ContainerID)
	if conf.ContainerID == "" {
		conf.ContainerID = defaultContainerID
	}
	c.containerID = conf.ContainerID
	c.logResults = conf.LogResults

	return c.CommonConfigure(senderManager, initConfig, data, source, provider)
}

// Run emits the placeholder metric with any current tagger tags for the configured container.
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	jobTags := c.tagsForContainer()
	tags := make([]string, 0, 1+len(jobTags))
	tags = append(tags, "container_id:"+c.containerID)
	tags = append(tags, jobTags...)

	sender.Gauge(metricName, 1, "", tags)
	sender.Commit()

	if c.logResults {
		log.Infof("%s emitted %s for container_id=%q tags=%v", CheckName, metricName, c.containerID, tags)
	}

	return nil
}

func (c *Check) tagsForContainer() []string {
	if c.tagger == nil {
		return nil
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, c.containerID)
	tags, err := c.tagger.Tag(entityID, taggertypes.OrchestratorCardinality)
	if err != nil {
		log.Warnf("%s could not read tagger tags for %q: %v", CheckName, entityID.String(), err)
		return nil
	}
	return tags
}
