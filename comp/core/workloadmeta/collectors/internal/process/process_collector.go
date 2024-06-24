// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process implements the local process collector for Workloadmeta.
package process

import (
	"context"
	"strconv"
	"time"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	processwlm "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "local-process-collector"
	componentName = "workloadmeta-process"
)

type collector struct {
	id      string
	store   workloadmeta.Component
	catalog workloadmeta.AgentType

	processDiffCh <-chan *processwlm.ProcessCacheDiff
}

// NewCollector returns a new local process collector provider and an error
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			catalog: workloadmeta.NodeAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) enabled() bool {
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return false
	}

	processChecksInCoreAgent := config.Datadog().GetBool("process_config.process_collection.enabled") &&
		config.Datadog().GetBool("process_config.run_in_core_agent.enabled")
	langDetectionEnabled := config.Datadog().GetBool("language_detection.enabled")

	return langDetectionEnabled && processChecksInCoreAgent
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !c.enabled() {
		return errors.NewDisabled(componentName, "core agent language detection not enabled")
	}

	c.store = store
	c.processDiffCh = processwlm.GetSharedWorkloadMetaExtractor(config.SystemProbe).ProcessCacheDiff()

	go c.stream(ctx)

	return nil
}

func (c *collector) stream(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
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
			cancel()
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
// The type of event is based whether a process was created or deleted since the last diff.
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
