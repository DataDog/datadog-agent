// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"errors"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	checkName    = "sbom"
	metricPeriod = 15 * time.Minute
)

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Config holds the container_image check configuration
type Config struct {
	ChunkSize                       int `yaml:"chunk_size"`
	NewSBOMMaxLatencySeconds        int `yaml:"new_sbom_max_latency_seconds"`
	ContainerPeriodicRefreshSeconds int `yaml:"periodic_refresh_seconds"`
	HostPeriodicRefreshSeconds      int `yaml:"host_periodic_refresh_seconds"`
}

type configValueRange struct {
	min          int
	max          int
	defaultValue int
}

var /* const */ (
	chunkSizeValueRange = &configValueRange{
		min:          1,
		max:          100,
		defaultValue: 1,
	}

	newSBOMMaxLatencySecondsValueRange = &configValueRange{
		min:          1,   // 1 s
		max:          300, // 5 min
		defaultValue: 30,  // 30 s
	}

	containerPeriodicRefreshSecondsValueRange = &configValueRange{
		min:          60,     // 1 min
		max:          604800, // 1 week
		defaultValue: 3600,   // 1h
	}

	hostPeriodicRefreshSecondsValueRange = &configValueRange{
		min:          60,        // 1 min
		max:          604800,    // 1 week
		defaultValue: 3600 * 24, // 1h
	}
)

func validateValue(val *int, valueRange *configValueRange) {
	if *val == 0 {
		*val = valueRange.defaultValue
	} else if *val < valueRange.min {
		*val = valueRange.min
	} else if *val > valueRange.max {
		*val = valueRange.max
	}
}

func (c *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	validateValue(&c.ChunkSize, chunkSizeValueRange)
	validateValue(&c.NewSBOMMaxLatencySeconds, newSBOMMaxLatencySecondsValueRange)
	validateValue(&c.ContainerPeriodicRefreshSeconds, containerPeriodicRefreshSecondsValueRange)
	validateValue(&c.HostPeriodicRefreshSeconds, hostPeriodicRefreshSecondsValueRange)

	return nil
}

// Check reports SBOM
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Store
	instance          *Config
	processor         *processor
	sender            sender.Sender
	stopCh            chan struct{}
}

// CheckFactory registers the sbom check
func CheckFactory() check.Check {
	return &Check{
		CheckBase:         core.NewCheckBase(checkName),
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		instance:          &Config{},
		stopCh:            make(chan struct{}),
	}
}

// Configure parses the check configuration and initializes the sbom check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if !ddConfig.Datadog.GetBool("sbom.enabled") {
		return errors.New("collection of SBOM is disabled")
	}

	if err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, config, source); err != nil {
		return err
	}

	if err := c.instance.Parse(config); err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	c.sender = sender
	sender.SetNoIndex(true)

	c.processor, err = newProcessor(c.workloadmetaStore, sender, c.instance.ChunkSize, time.Duration(c.instance.NewSBOMMaxLatencySeconds)*time.Second, ddConfig.Datadog.GetBool("sbom.host.enabled"))
	if err != nil {
		return err
	}

	return nil
}

// Run starts the sbom check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	imgEventsCh := c.workloadmetaStore.Subscribe(
		checkName,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{
				workloadmeta.KindContainerImageMetadata,
				workloadmeta.KindContainer,
			},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeAll,
		),
	)

	// Trigger an initial scan on host
	c.processor.processHostRefresh()

	c.sendUsageMetrics()

	containerPeriodicRefreshTicker := time.NewTicker(time.Duration(c.instance.ContainerPeriodicRefreshSeconds) * time.Second)
	defer containerPeriodicRefreshTicker.Stop()

	hostPeriodicRefreshTicker := time.NewTicker(time.Duration(c.instance.HostPeriodicRefreshSeconds) * time.Second)
	defer hostPeriodicRefreshTicker.Stop()

	metricTicker := time.NewTicker(metricPeriod)
	defer metricTicker.Stop()

	for {
		select {
		case eventBundle := <-imgEventsCh:
			c.processor.processContainerImagesEvents(eventBundle)
		case <-containerPeriodicRefreshTicker.C:
			c.processor.processContainerImagesRefresh(c.workloadmetaStore.ListImages())
		case <-hostPeriodicRefreshTicker.C:
			c.processor.processHostRefresh()
		case <-metricTicker.C:
			c.sendUsageMetrics()
		case <-c.stopCh:
			c.processor.stop()
			return nil
		}
	}
}

func (c *Check) sendUsageMetrics() {
	c.sender.Count("datadog.agent.sbom.container_images.running", 1.0, "", nil)

	if ddConfig.Datadog.GetBool("sbom.host.enabled") {
		c.sender.Count("datadog.agent.sbom.hosts.running", 1.0, "", nil)
	}

	c.sender.Commit()
}

// Stop stops the sbom check
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0. It makes sbom a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}
