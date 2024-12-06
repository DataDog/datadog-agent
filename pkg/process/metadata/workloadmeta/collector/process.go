// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package collector

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	workloadmetaExtractor "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorId       = "local-process"
	cacheValidityNoRT = 2 * time.Second
)

// NewProcessCollector creates a new process collector.
func NewProcessCollector(coreConfig, sysProbeConfig pkgconfigmodel.Reader) *Collector {
	wlmExtractor := workloadmetaExtractor.NewWorkloadMetaExtractor(sysProbeConfig)

	processData := checks.NewProcessData(coreConfig)
	processData.Register(wlmExtractor)

	return &Collector{
		ddConfig:        coreConfig,
		wlmExtractor:    wlmExtractor,
		grpcServer:      workloadmetaExtractor.NewGRPCServer(coreConfig, wlmExtractor),
		processData:     processData,
		collectionClock: clock.New(),
		pidToCid:        make(map[int]string),
	}
}

// Collector collects processes to send to the remote process collector in the core agent.
// It is only intended to be used when language detection is enabled, and the process check is disabled.
type Collector struct {
	ddConfig pkgconfigmodel.Reader

	processData *checks.ProcessData

	wlmExtractor *workloadmetaExtractor.WorkloadMetaExtractor
	grpcServer   *workloadmetaExtractor.GRPCServer

	pidToCid map[int]string

	collectionClock   clock.Clock
	containerProvider proccontainers.ContainerProvider
}

// Start will start the collector
func (c *Collector) Start(ctx context.Context, store workloadmeta.Component) error {
	err := c.grpcServer.Start()
	if err != nil {
		return err
	}

	collectionTicker := c.collectionClock.Ticker(
		c.ddConfig.GetDuration("workloadmeta.local_process_collector.collection_interval"),
	)

	if c.containerProvider == nil {
		sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()

		if err != nil {
			return err
		}

		c.containerProvider = sharedContainerProvider
	}

	go c.run(ctx, c.containerProvider, collectionTicker)

	return nil
}

func (c *Collector) run(ctx context.Context, containerProvider proccontainers.ContainerProvider, collectionTicker *clock.Ticker) {
	defer c.grpcServer.Stop()
	defer collectionTicker.Stop()

	log.Info("Starting local process collection server")

	for {
		select {
		case <-collectionTicker.C:
			// This ensures all processes are mapped correctly to a container and not just the principal process
			c.pidToCid = containerProvider.GetPidToCid(cacheValidityNoRT)
			c.wlmExtractor.SetLastPidToCid(c.pidToCid)
			err := c.processData.Fetch()
			if err != nil {
				log.Error("Error fetching process data:", err)
			}
		case <-ctx.Done():
			log.Infof("The %s collector has stopped", collectorId)
			return
		}
	}
}

// Enabled checks to see if we should enable the local process collector.
// Since it's job is to collect processes when the process check is disabled, we only enable it when `process_config.process_collection.enabled` == false
// Additionally, if the remote process collector is not enabled in the core agent, there is no reason to collect processes. Therefore, we check `language_detection.enabled`.
// We also check `process_config.run_in_core_agent.enabled` because this collector should only be used when the core agent collector is not running.
// Finally, we only want to run this collector in the process agent, so if we're running as anything else we should disable the collector.
func Enabled(cfg pkgconfigmodel.Reader) bool {
	if cfg.GetBool("process_config.process_collection.enabled") {
		return false
	}

	if !cfg.GetBool("language_detection.enabled") {
		return false
	}

	if cfg.GetBool("process_config.run_in_core_agent.enabled") {
		return false
	}

	return true
}
