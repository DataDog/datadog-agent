// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sbom

import (
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	checkName = "sbom"
)

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Config holds the container_image check configuration
type Config struct {
	ChunkSize                int `yaml:"chunk_size"`
	NewSBOMMaxLatencySeconds int `yaml:"new_sbom_max_latency_seconds"`
	PeriodicRefreshSeconds   int `yaml:"periodic_refresh_seconds"`
}

type configValueRange struct {
	min      int
	max      int
	default_ int
}

var /* const */ (
	chunkSizeValueRange = &configValueRange{
		min:      1,
		max:      100,
		default_: 1,
	}

	newSBOMMaxLatencySecondsValueRange = &configValueRange{
		min:      1,   // 1 s
		max:      300, // 5 min
		default_: 30,  // 30 s
	}

	periodicRefreshSecondsValueRange = &configValueRange{
		min:      60,     // 1 min
		max:      604800, // 1 week
		default_: 3600,   // 1h
	}
)

func validateValue(val *int, range_ *configValueRange) {
	if *val == 0 {
		*val = range_.default_
	} else if *val < range_.min {
		*val = range_.min
	} else if *val > range_.max {
		*val = range_.max
	}
}

func (c *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	validateValue(&c.ChunkSize, chunkSizeValueRange)
	validateValue(&c.NewSBOMMaxLatencySeconds, newSBOMMaxLatencySecondsValueRange)
	validateValue(&c.PeriodicRefreshSeconds, periodicRefreshSecondsValueRange)

	return nil
}

// Check reports SBOM
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Store
	instance          *Config
	processor         *processor
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
func (c *Check) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, config, source); err != nil {
		return err
	}

	if err := c.instance.Parse(config); err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	c.processor = newProcessor(c.workloadmetaStore, sender, c.instance.ChunkSize, time.Duration(c.instance.NewSBOMMaxLatencySeconds)*time.Second)

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

	imgRefreshTicker := time.NewTicker(time.Duration(c.instance.PeriodicRefreshSeconds) * time.Second)

	for {
		select {
		case eventBundle := <-imgEventsCh:
			c.processor.processEvents(eventBundle)
		case <-imgRefreshTicker.C:
			c.processor.processRefresh(c.workloadmetaStore.ListImages())
		case <-c.stopCh:
			c.processor.stop()
			return nil
		}
	}
}

// Stop stops the sbom check
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0. It makes sbom a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}
