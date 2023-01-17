// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"errors"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	checkName           = "container_lifecycle"
	maxChunkSize        = 100
	defaultPollInterval = 10
)

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Config holds the container_lifecycle check configuration
type Config struct {
	chunkSize    int `yaml:"chunk_size"`
	pollInterval int `yaml:"poll_interval_seconds"`
}

// Parse parses the container_lifecycle check config and set default values
func (c *Config) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Check reports container lifecycle events
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Store
	instance          *Config
	processor         *processor
	stopCh            chan struct{}
}

// Configure parses the check configuration and initializes the container_lifecycle check
func (c *Check) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if !ddConfig.Datadog.GetBool("container_lifecycle.enabled") {
		return errors.New("collection of container lifecycle events is disabled")
	}

	var err error

	err = c.CommonConfigure(integrationConfigDigest, initConfig, config, source)
	if err != nil {
		return err
	}

	err = c.instance.Parse(config)
	if err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	if c.instance.chunkSize <= 0 || c.instance.chunkSize > maxChunkSize {
		c.instance.chunkSize = maxChunkSize
	}

	if c.instance.pollInterval <= 0 {
		c.instance.pollInterval = defaultPollInterval
	}

	c.processor = newProcessor(sender, c.instance.chunkSize)

	return nil
}

// Run starts the container_lifecycle check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	contEventsCh := c.workloadmetaStore.Subscribe(
		checkName+"-cont",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{workloadmeta.KindContainer},
			workloadmeta.SourceRuntime,
			workloadmeta.EventTypeUnset,
		),
	)

	podEventsCh := c.workloadmetaStore.Subscribe(
		checkName+"-pod",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{workloadmeta.KindKubernetesPod},
			workloadmeta.SourceNodeOrchestrator,
			workloadmeta.EventTypeUnset,
		),
	)

	pollInterval := time.Duration(c.instance.pollInterval) * time.Second

	processorCtx, stopProcessor := context.WithCancel(context.Background())
	c.processor.start(processorCtx, pollInterval)

	for {
		select {
		case eventBundle := <-contEventsCh:
			c.processor.processEvents(eventBundle)
		case eventBundle := <-podEventsCh:
			c.processor.processEvents(eventBundle)
		case <-c.stopCh:
			stopProcessor()
			return nil
		}
	}
}

// Stop stops the container_lifecycle check
func (c *Check) Stop() { close(c.stopCh) }

// Interval returns 0, it makes container_lifecycle a long-running check
func (c *Check) Interval() time.Duration { return 0 }

// CheckFactory registers the container_lifecycle check
func CheckFactory() check.Check {
	return &Check{
		CheckBase:         core.NewCheckBase(checkName),
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		instance:          &Config{},
		stopCh:            make(chan struct{}),
	}
}
