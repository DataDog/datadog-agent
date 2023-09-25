// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

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
	checkName = "container_image"
)

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Config holds the container_image check configuration
type Config struct {
	ChunkSize                  int `yaml:"chunk_size"`
	NewImagesMaxLatencySeconds int `yaml:"new_images_max_latency_seconds"`
	PeriodicRefreshSeconds     int `yaml:"periodic_refresh_seconds"`
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
		defaultValue: 10,
	}

	newImagesMaxLatencySecondsValueRange = &configValueRange{
		min:          1,   // 1 s
		max:          300, // 5 min
		defaultValue: 30,  // 30 s
	}

	periodicRefreshSecondsValueRange = &configValueRange{
		min:          60,    // 1 min
		max:          86400, // 1 day
		defaultValue: 300,   // 5 min
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
	validateValue(&c.NewImagesMaxLatencySeconds, newImagesMaxLatencySecondsValueRange)
	validateValue(&c.PeriodicRefreshSeconds, periodicRefreshSecondsValueRange)

	return nil
}

// Check reports container images
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Store
	instance          *Config
	processor         *processor
	stopCh            chan struct{}
}

// CheckFactory registers the container_image check
func CheckFactory() check.Check {
	return &Check{
		CheckBase:         core.NewCheckBase(checkName),
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		instance:          &Config{},
		stopCh:            make(chan struct{}),
	}
}

// Configure parses the check configuration and initializes the container_image check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if !ddConfig.Datadog.GetBool("container_image.enabled") {
		return errors.New("collection of container images is disabled")
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

	c.processor = newProcessor(sender, c.instance.ChunkSize, time.Duration(c.instance.NewImagesMaxLatencySeconds)*time.Second)

	return nil
}

// Run starts the container_image check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	imgEventsCh := c.workloadmetaStore.Subscribe(
		checkName,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{workloadmeta.KindContainerImageMetadata},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeSet, // We don’t care about images removal because we just have to wait for them to expire on BE side once we stopped refreshing them periodically.
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

// Stop stops the container_image check
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0. It makes container_image a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}
