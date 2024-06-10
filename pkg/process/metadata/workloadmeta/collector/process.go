// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package collector

import (
	"context"

	"github.com/benbjohnson/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	workloadmetaExtractor "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const collectorId = "local-process"

// NewProcessCollector creates a new process collector.
func NewProcessCollector(coreConfig, sysProbeConfig config.Reader) *Collector {
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
	ddConfig config.Reader

	processData *checks.ProcessData

	wlmExtractor *workloadmetaExtractor.WorkloadMetaExtractor
	grpcServer   *workloadmetaExtractor.GRPCServer

	pidToCid map[int]string

	collectionClock clock.Clock
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

	filterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindContainer},
		Source:    workloadmeta.SourceAll,
		EventType: workloadmeta.EventTypeAll,
	}
	filter := workloadmeta.NewFilter(&filterParams)
	containerEvt := store.Subscribe(collectorId, workloadmeta.NormalPriority, filter)

	go c.run(ctx, store, containerEvt, collectionTicker)

	return nil
}

func (c *Collector) run(ctx context.Context, store workloadmeta.Component, containerEvt chan workloadmeta.EventBundle, collectionTicker *clock.Ticker) {
	defer c.grpcServer.Stop()
	defer store.Unsubscribe(containerEvt)
	defer collectionTicker.Stop()

	log.Info("Starting local process collection server")

	for {
		select {
		case evt, ok := <-containerEvt:
			if !ok {
				log.Infof("The %s collector has stopped, workloadmeta channel is closed", collectorId)
				return
			}
			c.handleContainerEvent(evt)
		case <-collectionTicker.C:
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

func (c *Collector) handleContainerEvent(evt workloadmeta.EventBundle) {
	defer evt.Acknowledge()

	for _, evt := range evt.Events {
		ent := evt.Entity.(*workloadmeta.Container)
		switch evt.Type {
		case workloadmeta.EventTypeSet:
			// Should be safe, even on windows because PID 0 is the idle process and therefore must always belong to the host
			if ent.PID != 0 {
				c.pidToCid[ent.PID] = ent.ID
			}
		case workloadmeta.EventTypeUnset:
			delete(c.pidToCid, ent.PID)
		}
	}

	c.wlmExtractor.SetLastPidToCid(c.pidToCid)
}

// Enabled checks to see if we should enable the local process collector.
// Since it's job is to collect processes when the process check is disabled, we only enable it when `process_config.process_collection.enabled` == false
// Additionally, if the remote process collector is not enabled in the core agent, there is no reason to collect processes. Therefore, we check `language_detection.enabled`
// Finally, we only want to run this collector in the process agent, so if we're running as anything else we should disable the collector.
func Enabled(cfg config.Reader) bool {
	if cfg.GetBool("process_config.process_collection.enabled") {
		return false
	}

	if !cfg.GetBool("language_detection.enabled") {
		return false
	}
	return true
}
