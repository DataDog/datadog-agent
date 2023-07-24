// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	workloadmetaExtractor "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const collectorId = "process-agent"

const collectionInterval = 1 * time.Minute

// Used for testing
var c *collector

func init() {
	// The process collector can run either when workloadmeta is in local or remote collector mode.
	workloadmeta.RegisterCollector(collectorId, newProcessCollector)
	workloadmeta.RegisterRemoteCollector(collectorId, newProcessCollector)
}

func newProcessCollector() workloadmeta.Collector {
	// TODO: Inject config.Datadog via fx once collectors are migrated to components.
	ddConfig := config.Datadog

	wlmExtractor := workloadmetaExtractor.NewWorkloadMetaExtractor(ddConfig)

	processData := checks.NewProcessData(ddConfig)
	processData.Register(wlmExtractor)

	c = &collector{
		ddConfig:        ddConfig,
		wlmExtractor:    wlmExtractor,
		grpcServer:      workloadmetaExtractor.NewGRPCServer(ddConfig, wlmExtractor),
		processData:     processData,
		collectionClock: clock.New(),
		pidToCid:        make(map[int]string),
	}
	return c
}

var _ workloadmeta.Collector = (*collector)(nil)

type collector struct {
	ddConfig config.ConfigReader

	processData *checks.ProcessData

	wlmExtractor *workloadmetaExtractor.WorkloadMetaExtractor
	grpcServer   *workloadmetaExtractor.GRPCServer

	pidToCid map[int]string

	collectionClock clock.Clock
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if enabled, err := Enabled(c.ddConfig); !enabled {
		return err
	}

	err := c.grpcServer.Start()
	if err != nil {
		return err
	}

	collectionTicker := c.collectionClock.Ticker(collectionInterval)

	filter := workloadmeta.NewFilter([]workloadmeta.Kind{workloadmeta.KindContainer}, workloadmeta.SourceAll, workloadmeta.EventTypeAll)
	containerEvt := store.Subscribe(collectorId, workloadmeta.NormalPriority, filter)
	go func() {
		defer c.grpcServer.Stop()
		defer store.Unsubscribe(containerEvt)
		defer collectionTicker.Stop()

		for {
			select {
			case evt := <-containerEvt:
				c.handleContainerEvent(evt)
			case <-collectionTicker.C:
				err := c.processData.Fetch()
				if err != nil {
					_ = log.Error("Error fetching process data:", err)
				}
			case <-ctx.Done():
				log.Infof("The %s collector has stopped", collectorId)
				return
			}
		}
	}()
	return nil
}

// Pull is unused at the moment used due to the short frequency in which it is called.
// In the future, we should use it to locally in workload-meta.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) handleContainerEvent(evt workloadmeta.EventBundle) {
	defer close(evt.Ch)

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

func Enabled(cfg config.ConfigReader) (bool, error) {
	if cfg.GetBool("process_config.process_collection.enabled") {
		return false, dderrors.NewDisabled(collectorId, "the process check is enabled")
	}

	if !cfg.GetBool("workloadmeta.remote_process_collector.enabled") {
		return false, dderrors.NewDisabled(collectorId, "the process collector is disabled")
	}
	return true, nil
}
