// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process implements the local process collector for Workloadmeta.
package process

import (
	"context"
	"strconv"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/errors"
	processwlm "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "local-process-collector"
	componentName     = "workloadmeta-process"
	cacheValidityNoRT = 2 * time.Second
)

type collector struct {
	id      string
	store   workloadmeta.Component
	catalog workloadmeta.AgentType

	wlmExtractor  *processwlm.WorkloadMetaExtractor
	processDiffCh <-chan *processwlm.ProcessCacheDiff

	// only used when process checks are disabled
	processData       *Data
	pidToCid          map[int]string
	collectionClock   clock.Clock
	containerProvider proccontainers.ContainerProvider
}

// NewCollector returns a new local process collector provider and an error.
// Currently, this is only used on Linux when language detection and run in core agent are enabled.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	wlmExtractor := processwlm.GetSharedWorkloadMetaExtractor(pkgconfigsetup.SystemProbe())
	processData := NewProcessData()
	processData.Register(wlmExtractor)

	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:              collectorID,
			catalog:         workloadmeta.NodeAgent,
			wlmExtractor:    wlmExtractor,
			processDiffCh:   wlmExtractor.ProcessCacheDiff(),
			processData:     processData,
			pidToCid:        make(map[int]string),
			collectionClock: clock.New(),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !util.LocalProcessCollectorIsEnabled() {
		return errors.NewDisabled(componentName, "language detection or core agent process collection is disabled")
	}

	c.store = store

	// If process collection is disabled, the collector will gather the basic process and container data
	// necessary for language detection.
	if !pkgconfigsetup.Datadog().GetBool("process_config.process_collection.enabled") {
		collectionTicker := c.collectionClock.Ticker(10 * time.Second)
		if c.containerProvider == nil {
			sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()

			if err != nil {
				return err
			}

			c.containerProvider = sharedContainerProvider
		}
		go c.collect(ctx, c.containerProvider, collectionTicker)
	}

	go c.stream(ctx)

	return nil
}

func (c *collector) collect(ctx context.Context, containerProvider proccontainers.ContainerProvider, collectionTicker *clock.Ticker) {
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()

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
			log.Infof("The %s collector has stopped", collectorID)
			return
		}
	}
}

func (c *collector) stream(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	health := health.RegisterLiveness(componentName)
	for {
		select {
		case <-health.C:

		case diff := <-c.processDiffCh:
			log.Debugf("Received process diff with %d creations and %d deletions", len(diff.Creation), len(diff.Deletion))
			events := transform(diff)
			c.store.Notify(events)

		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		}
	}
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

// transform converts a ProcessCacheDiff into a list of CollectorEvents.
// The type of event is based on whether a process was created or deleted since the last diff.
func transform(diff *processwlm.ProcessCacheDiff) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(diff.Creation)+len(diff.Deletion))

	for _, creation := range diff.Creation {
		events = append(events, workloadmeta.CollectorEvent{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(creation.Pid)),
				},
				ContainerID:  creation.ContainerId,
				NsPid:        creation.NsPid,
				CreationTime: time.UnixMilli(creation.CreationTime),
				Language:     creation.Language,
			},
			Source: workloadmeta.SourceLocalProcessCollector,
		})
	}

	for _, deletion := range diff.Deletion {
		events = append(events, workloadmeta.CollectorEvent{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(deletion.Pid)),
				},
			},
			Source: workloadmeta.SourceLocalProcessCollector,
		})
	}

	return events
}
