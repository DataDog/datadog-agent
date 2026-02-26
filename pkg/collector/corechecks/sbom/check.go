// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package sbom

import (
	"errors"
	"runtime"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName    = "sbom"
	metricPeriod = 15 * time.Minute
)

// Config holds the container_image check configuration
type Config struct {
	ChunkSize                       int `yaml:"chunk_size"`
	NewSBOMMaxLatencySeconds        int `yaml:"new_sbom_max_latency_seconds"`
	ContainerPeriodicRefreshSeconds int `yaml:"periodic_refresh_seconds"`
	HostPeriodicRefreshSeconds      int `yaml:"host_periodic_refresh_seconds"`
	HostHeartbeatValiditySeconds    int `yaml:"host_heartbeat_validity_seconds"`
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
		min:          1,   // 1 seconds
		max:          300, // 5 min
		defaultValue: 30,  // 30 seconds
	}

	containerPeriodicRefreshSecondsValueRange = &configValueRange{
		min:          60,     // 1 min
		max:          604800, // 1 week
		defaultValue: 3600,   // 1 hour
	}

	hostPeriodicRefreshSecondsValueRange = &configValueRange{
		min:          60,     // 1 min
		max:          604800, // 1 week
		defaultValue: 3600,   // 1 hour
	}

	hostHeartbeatValiditySeconds = &configValueRange{
		min:          60,        // 1 min
		max:          604800,    // 1 week
		defaultValue: 3600 * 24, // 1 day
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
	validateValue(&c.NewSBOMMaxLatencySeconds, newSBOMMaxLatencySecondsValueRange)
	validateValue(&c.ContainerPeriodicRefreshSeconds, containerPeriodicRefreshSecondsValueRange)
	validateValue(&c.HostPeriodicRefreshSeconds, hostPeriodicRefreshSecondsValueRange)
	validateValue(&c.HostHeartbeatValiditySeconds, hostHeartbeatValiditySeconds)

	return nil
}

// Check reports SBOM
type Check struct {
	core.CheckBase
	workloadmetaStore workloadmeta.Component
	filterStore       workloadfilter.Component
	tagger            tagger.Component
	instance          *Config
	processor         *processor
	sender            sender.Sender
	stopCh            chan struct{}
	cfg               config.Component
}

// Factory returns a new check factory
func Factory(store workloadmeta.Component, filterStore workloadfilter.Component, cfg config.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase:         core.NewCheckBase(CheckName),
			workloadmetaStore: store,
			filterStore:       filterStore,
			tagger:            tagger,
			instance:          &Config{},
			stopCh:            make(chan struct{}),
			cfg:               cfg,
		})
	})
}

// Configure parses the check configuration and initializes the sbom check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if !c.cfg.GetBool("sbom.enabled") {
		return errors.New("collection of SBOM is disabled")
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

	c.sender = sender

	if c.processor, err = newProcessor(
		c.workloadmetaStore,
		c.filterStore,
		sender,
		c.tagger,
		c.cfg,
		c.instance.ChunkSize,
		time.Duration(c.instance.NewSBOMMaxLatencySeconds)*time.Second,
		time.Duration(c.instance.HostHeartbeatValiditySeconds)*time.Second); err != nil {
		return err
	}

	return nil
}

// Run starts the sbom check
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()

	imgEventsCh := c.workloadmetaStore.Subscribe(
		CheckName,
		workloadmeta.NormalPriority,
		filter,
	)

	// Trigger an initial scan on host. This channel is buffered to avoid blocking the scanner
	// if the processor is not ready to receive the result yet. This channel should not be closed,
	// it is sent as part of every scan request. When the main context terminates, both references will
	// be dropped and the scanner will be garbage collected.
	hostSbomChan := make(chan sbom.ScanResult) // default value to listen to nothing
	if collectors.GetHostScanner() != nil && collectors.GetHostScanner().Channel() != nil {
		hostSbomChan = collectors.GetHostScanner().Channel()
	}
	c.processor.triggerHostScan()

	c.sendUsageMetrics()

	containerRefreshPeriod := time.Duration(c.instance.ContainerPeriodicRefreshSeconds) * time.Second
	var containerRefresher containerPeriodicRefresher
	if c.cfg.GetBool("sbom.container_image.use_spread_refresher") {
		containerRefresher = newSpreadRefresher(containerRefreshPeriod, c.workloadmetaStore, c.processor)
	} else {
		containerRefresher = newBatchRefresher(containerRefreshPeriod, c.workloadmetaStore, c.processor)
	}
	defer containerRefresher.stop()

	procfsSbomChan := make(chan sbom.ScanResult) // default value to listen to nothing
	if collectors.GetProcfsScanner() != nil && collectors.GetProcfsScanner().Channel() != nil {
		procfsSbomChan = collectors.GetProcfsScanner().Channel()
	}

	hostPeriodicRefreshTicker := time.NewTicker(time.Duration(c.instance.HostPeriodicRefreshSeconds) * time.Second)
	defer hostPeriodicRefreshTicker.Stop()

	metricTicker := time.NewTicker(metricPeriod)
	defer metricTicker.Stop()

	defer c.processor.stop()
	for {
		select {
		case eventBundle, ok := <-imgEventsCh:
			if !ok {
				return nil
			}
			c.processor.processContainerImagesEvents(eventBundle)
		case scanResult, ok := <-hostSbomChan:
			if !ok {
				return nil
			}
			c.processor.processHostScanResult(scanResult)
		case scanResult, ok := <-procfsSbomChan:
			if !ok {
				return nil
			}
			c.processor.processProcfsScanResult(scanResult)
		case <-containerRefresher.tick():
			containerRefresher.step()
		case <-hostPeriodicRefreshTicker.C:
			c.processor.triggerHostScan()
		case <-metricTicker.C:
			c.sendUsageMetrics()
		case <-c.stopCh:
			return nil
		}
	}
}

func (c *Check) sendUsageMetrics() {
	if c.cfg.GetBool("sbom.container_image.enabled") {
		c.sender.Count("datadog.agent.sbom.container_images.running", 1.0, "", nil)
	}

	if c.cfg.GetBool("sbom.host.enabled") {
		c.sender.Count("datadog.agent.sbom.hosts.running", 1.0, "", []string{"os:" + runtime.GOOS})
	}

	c.sender.Commit()
}

// Cancel stops the sbom check
func (c *Check) Cancel() {
	close(c.stopCh)
}

// Interval returns 0. It makes sbom a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}
