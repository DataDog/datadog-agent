// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"fmt"
	"time"

	apievents "github.com/containerd/containerd/api/events"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "containerd"
	componentName = "workloadmeta-containerd"

	containerCreationTopic = "/containers/create"
	containerUpdateTopic   = "/containers/update"
	containerDeletionTopic = "/containers/delete"

	// These are not all the task-related topics, but enough to detect changes
	// in the state of the container (only need to know if it's running or not).

	TaskStartTopic   = "/tasks/start"
	TaskOOMTopic     = "/tasks/oom"
	TaskExitTopic    = "/tasks/exit"
	TaskDeleteTopic  = "/tasks/delete"
	TaskPausedTopic  = "/tasks/paused"
	TaskResumedTopic = "/tasks/resumed"
)

// containerLifecycleFilters allows subscribing to containers lifecycle updates only.
var containerLifecycleFilters = []string{
	fmt.Sprintf(`topic==%q`, containerCreationTopic),
	fmt.Sprintf(`topic==%q`, containerUpdateTopic),
	fmt.Sprintf(`topic==%q`, containerDeletionTopic),
	fmt.Sprintf(`topic==%q`, TaskStartTopic),
	fmt.Sprintf(`topic==%q`, TaskOOMTopic),
	fmt.Sprintf(`topic==%q`, TaskExitTopic),
	fmt.Sprintf(`topic==%q`, TaskDeleteTopic),
	fmt.Sprintf(`topic==%q`, TaskPausedTopic),
	fmt.Sprintf(`topic==%q`, TaskResumedTopic),
}

type collector struct {
	store            workloadmeta.Store
	containerdClient cutil.ContainerdItf
	eventsChan       <-chan *containerdevents.Envelope
	errorsChan       <-chan error
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Containerd) {
		return errors.NewDisabled(componentName, "Agent is not running on containerd")
	}

	c.store = store

	var err error
	c.containerdClient, err = cutil.GetContainerdUtil()
	if err != nil {
		return err
	}

	eventsCtx, cancelEvents := context.WithCancel(ctx)
	c.eventsChan, c.errorsChan = c.containerdClient.GetEvents().Subscribe(eventsCtx, containerLifecycleFilters...)

	err = c.generateEventsFromContainerList(ctx)
	if err != nil {
		cancelEvents()
		return err
	}

	go func() {
		defer cancelEvents()
		c.stream(ctx)
	}()

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	return nil
}

func (c *collector) stream(ctx context.Context) {
	healthHandle := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)

	for {
		select {
		case <-healthHandle.C:

		case ev := <-c.eventsChan:
			if err := c.handleEvent(ctx, ev, c.containerdClient); err != nil {
				log.Warnf(err.Error())
			}

		case err := <-c.errorsChan:
			if err != nil {
				log.Errorf("stopping collection: %s", err)
			}
			cancel()
			return

		case <-ctx.Done():
			err := healthHandle.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			cancel()
			return
		}
	}
}

func (c *collector) generateEventsFromContainerList(ctx context.Context) error {
	containerdEvents, err := c.generateInitialEvents()
	if err != nil {
		return err
	}

	events := make([]workloadmeta.CollectorEvent, 0, len(containerdEvents))
	for _, containerdEvent := range containerdEvents {
		ev, err := buildCollectorEvent(ctx, &containerdEvent, c.containerdClient)
		if err != nil {
			log.Warnf(err.Error())
			continue
		}

		events = append(events, ev)
	}

	if len(events) > 0 {
		c.store.Notify(events)
	}

	return nil
}

func (c *collector) generateInitialEvents() ([]containerdevents.Envelope, error) {
	var events []containerdevents.Envelope

	containers, err := c.containerdClient.Containers()
	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		eventEncoded, err := proto.Marshal(&apievents.ContainerCreate{
			ID: container.ID(),
		})
		if err != nil {
			return nil, err
		}

		events = append(events, containerdevents.Envelope{
			Timestamp: time.Now(),
			Topic:     containerCreationTopic,
			Event: &types.Any{
				TypeUrl: "containerd.events.ContainerCreate",
				Value:   eventEncoded,
			},
		})
	}

	return events, nil
}

func (c *collector) handleEvent(ctx context.Context, containerdEvent *containerdevents.Envelope, containerdClient cutil.ContainerdItf) error {
	workloadmetaEvent, err := buildCollectorEvent(ctx, containerdEvent, containerdClient)
	if err != nil {
		return err
	}

	c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})

	return nil
}
