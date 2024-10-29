// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package containerimage implements the container image check.
package containerimage

import (
	"errors"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName = "container_image"
)

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

// Parse parses the configuration
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
	workloadmetaStore workloadmeta.Component
	tagger            tagger.Component
	instance          *Config
	processor         *processor
	stopCh            chan struct{}
}

// Factory returns a new check factory
func Factory(store workloadmeta.Component, tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase:         core.NewCheckBase(CheckName),
			workloadmetaStore: store,
			instance:          &Config{},
			stopCh:            make(chan struct{}),
			tagger:            tagger,
		})
	})
}

// Configure parses the check configuration and initializes the container_image check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if !pkgconfigsetup.Datadog().GetBool("container_image.enabled") {
		return errors.New("collection of container images is disabled")
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := c.instance.Parse(config); err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	c.processor = newProcessor(sender, c.instance.ChunkSize, time.Duration(c.instance.NewImagesMaxLatencySeconds)*time.Second, c.tagger)

	return nil
}

// Run starts the container_image check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	filter := workloadmeta.NewFilterBuilder().
		SetEventType(workloadmeta.EventTypeSet). // We don’t care about images removal because we just have to wait for them to expire on BE side once we stopped refreshing them periodically.
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()

	imgEventsCh := c.workloadmetaStore.Subscribe(
		CheckName,
		workloadmeta.NormalPriority,
		filter,
	)

	imgRefreshTicker := time.NewTicker(time.Duration(c.instance.PeriodicRefreshSeconds) * time.Second)
	defer imgRefreshTicker.Stop()
	defer c.processor.stop()
	for {
		select {
		case eventBundle, ok := <-imgEventsCh:
			if !ok {
				return nil
			}
			c.processor.processEvents(eventBundle)
		case <-imgRefreshTicker.C:
			c.processor.processRefresh(c.workloadmetaStore.ListImages())
		case <-c.stopCh:
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
