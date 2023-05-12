// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/containerd/containerd"
	containerdevents "github.com/containerd/containerd/events"

	"github.com/DataDog/datadog-agent/pkg/config"
	agentErrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "containerd"
	componentName = "workloadmeta-containerd"

	containerCreationTopic = "/containers/create"
	containerUpdateTopic   = "/containers/update"
	containerDeletionTopic = "/containers/delete"

	imageCreationTopic = "/images/create"
	imageUpdateTopic   = "/images/update"
	imageDeletionTopic = "/images/delete"

	// These are not all the task-related topics, but enough to detect changes
	// in the state of the container (only need to know if it's running or not).

	// TaskStartTopic represents task start events
	TaskStartTopic = "/tasks/start"
	// TaskOOMTopic represents task oom events
	TaskOOMTopic = "/tasks/oom"
	// TaskExitTopic represents task exit events
	TaskExitTopic = "/tasks/exit"
	// TaskDeleteTopic represents task delete events
	TaskDeleteTopic = "/tasks/delete"
	// TaskPausedTopic represents task paused events
	TaskPausedTopic = "/tasks/paused"
	// TaskResumedTopic represents task resumed events
	TaskResumedTopic = "/tasks/resumed"
)

// containerdTopics includes the containerd topics that we want to subscribe to
var containerdTopics = []string{
	containerCreationTopic,
	containerUpdateTopic,
	containerDeletionTopic,
	imageCreationTopic,
	imageUpdateTopic,
	imageDeletionTopic,
	TaskStartTopic,
	TaskOOMTopic,
	TaskExitTopic,
	TaskDeleteTopic,
	TaskPausedTopic,
	TaskResumedTopic,
}

type exitInfo struct {
	exitCode *uint32
	exitTS   time.Time
}

type collector struct {
	store                  workloadmeta.Store
	containerdClient       cutil.ContainerdItf
	filterPausedContainers *containers.Filter
	eventsChan             <-chan *containerdevents.Envelope
	errorsChan             <-chan error

	// Container exit info (mainly exit code and exit timestamp) are attached to the corresponding task events.
	// contToExitInfo caches the exit info of a task to enrich the container deletion event when it's received later.
	contToExitInfo map[string]*exitInfo

	knownImages *knownImages

	// Images are updated from 2 goroutines: the one that handles containerd
	// events, and the one that extracts SBOMS.
	// This mutex is used to handle images one at a time to avoid
	// inconsistencies like trying to set an SBOM for an image that is being
	// deleted.
	handleImagesMut sync.Mutex

	// SBOM Scanning
	sbomScanner *scanner.Scanner // nolint: unused
	scanOptions sbom.ScanOptions // nolint: unused
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{
			contToExitInfo: make(map[string]*exitInfo),
			knownImages:    newKnownImages(),
		}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Containerd) {
		return agentErrors.NewDisabled(componentName, "Agent is not running on containerd")
	}

	c.store = store

	var err error
	c.containerdClient, err = cutil.NewContainerdUtil()
	if err != nil {
		return err
	}

	if err = c.startSBOMCollection(ctx); err != nil {
		return err
	}

	c.filterPausedContainers, err = containers.GetPauseContainerFilter()
	if err != nil {
		return err
	}

	eventsCtx, cancelEvents := context.WithCancel(ctx)
	c.eventsChan, c.errorsChan = c.containerdClient.GetEvents().Subscribe(eventsCtx, subscribeFilters()...)

	err = c.notifyInitialEvents(ctx)
	if err != nil {
		cancelEvents()
		return err
	}

	go func() {
		defer func() {
			if errClose := c.containerdClient.Close(); errClose != nil {
				log.Warnf("Error when closing containerd connection: %s", errClose)
			}
		}()
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
			if err := c.handleEvent(ctx, ev); err != nil {
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

func (c *collector) notifyInitialEvents(ctx context.Context) error {
	var containerEvents []workloadmeta.CollectorEvent

	namespaces, err := cutil.NamespacesToWatch(ctx, c.containerdClient)
	if err != nil {
		return err
	}

	for _, namespace := range namespaces {
		nsContainerEvents, err := c.generateInitialContainerEvents(namespace)
		if err != nil {
			return err
		}
		containerEvents = append(containerEvents, nsContainerEvents...)

		if imageMetadataCollectionIsEnabled() {
			if err := c.notifyInitialImageEvents(ctx, namespace); err != nil {
				return err
			}
		}
	}

	if len(containerEvents) > 0 {
		c.store.Notify(containerEvents)
	}

	return nil
}

func (c *collector) generateInitialContainerEvents(namespace string) ([]workloadmeta.CollectorEvent, error) {
	var events []workloadmeta.CollectorEvent

	existingContainers, err := c.containerdClient.Containers(namespace)
	if err != nil {
		return nil, err
	}

	for _, container := range existingContainers {
		// if ignoreContainer returns an error, keep the container
		// regardless.  it might've been because of network errors, so
		// it's better to keep a container we should've ignored than
		// ignoring a container we should've kept
		ignore, err := c.ignoreContainer(namespace, container)
		if err != nil {
			log.Debugf("Error while deciding to ignore event %s, keeping it: %s", container.ID(), err)
		} else if ignore {
			continue
		}

		ev, err := createSetEvent(container, namespace, c.containerdClient)
		if err != nil {
			log.Warnf(err.Error())
			continue
		}

		events = append(events, ev)
	}

	return events, nil
}

func (c *collector) notifyInitialImageEvents(ctx context.Context, namespace string) error {
	existingImages, err := c.containerdClient.ListImages(namespace)
	if err != nil {
		return err
	}

	for _, image := range existingImages {
		if err := c.notifyEventForImage(ctx, namespace, image, nil); err != nil {
			log.Warnf("error getting information for image with name %q: %s", image.Name(), err.Error())
			continue
		}
	}

	return nil
}

func (c *collector) handleEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) error {
	if isImageTopic(containerdEvent.Topic) {
		return c.handleImageEvent(ctx, containerdEvent)
	}

	return c.handleContainerEvent(ctx, containerdEvent)
}

func (c *collector) handleContainerEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) error {
	containerID, container, err := c.extractContainerFromEvent(ctx, containerdEvent)
	if err != nil {
		return fmt.Errorf("cannot extract container from event: %w", err)
	}

	if container != nil {
		ignore, err := c.ignoreContainer(containerdEvent.Namespace, container)
		if err != nil {
			log.Debugf("Error while deciding to ignore event %s, keeping it: %s", container.ID(), err)
		} else if ignore {
			return nil
		}
	}

	workloadmetaEvent, err := c.buildCollectorEvent(containerdEvent, containerID, container)
	if err != nil {
		if errors.Is(err, errNoContainer) {
			log.Debugf("No event could be built as container is nil, skipping event. CID: %s, event: %+v", containerID, containerdEvent)
			return nil
		}

		return fmt.Errorf("cannot build collector event: %w", err)
	}

	c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})

	return nil
}

// extractContainerFromEvent extracts a container ID from an event, and
// queries for a containerd.Container object. The Container object will always
// be missing in a delete event, so that's why we return a separate ID and not
// just an object.
func (c *collector) extractContainerFromEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) (string, containerd.Container, error) {
	var (
		containerID string
		hasID       bool
	)

	switch containerdEvent.Topic {
	case containerCreationTopic, containerUpdateTopic, containerDeletionTopic:
		containerID, hasID = containerdEvent.Field([]string{"event", "id"})
		if !hasID {
			return "", nil, fmt.Errorf("missing ID in containerd event")
		}

	case TaskStartTopic, TaskOOMTopic, TaskPausedTopic, TaskResumedTopic, TaskExitTopic, TaskDeleteTopic:
		containerID, hasID = containerdEvent.Field([]string{"event", "container_id"})
		if !hasID {
			return "", nil, fmt.Errorf("missing ID in containerd event")
		}

	default:
		return "", nil, fmt.Errorf("unknown action type %s, ignoring", containerdEvent.Topic)
	}

	// ignore NotFound errors, since they happen for every deleted
	// container, but these events still need to be handled
	container, err := c.containerdClient.ContainerWithContext(ctx, containerdEvent.Namespace, containerID)
	if err != nil && !agentErrors.IsNotFound(err) {
		return "", nil, err
	}

	return containerID, container, nil
}

// ignoreContainer returns whether a containerd event should be ignored.
// The ignored events are the ones that refer to a "pause" container.
func (c *collector) ignoreContainer(namespace string, container containerd.Container) (bool, error) {
	isSandbox, err := c.containerdClient.IsSandbox(namespace, container)
	if err != nil {
		return false, err
	}

	if isSandbox {
		return true, nil
	}

	info, err := c.containerdClient.Info(namespace, container)
	if err != nil {
		return false, err
	}

	// Only the image name is relevant to exclude paused containers
	return c.filterPausedContainers.IsExcluded(nil, "", info.Image, ""), nil
}

func subscribeFilters() []string {
	var filters []string

	for _, topic := range containerdTopics {
		if isImageTopic(topic) && !imageMetadataCollectionIsEnabled() {
			continue
		}

		filters = append(filters, fmt.Sprintf(`topic==%q`, topic))
	}

	return cutil.FiltersWithNamespaces(filters)
}

func (c *collector) getExitInfo(id string) *exitInfo {
	return c.contToExitInfo[id]
}

func (c *collector) deleteExitInfo(id string) {
	delete(c.contToExitInfo, id)
}

func (c *collector) cacheExitInfo(id string, exitCode *uint32, exitTS time.Time) {
	c.contToExitInfo[id] = &exitInfo{
		exitTS:   exitTS,
		exitCode: exitCode,
	}
}

func imageMetadataCollectionIsEnabled() bool {
	return config.Datadog.GetBool("container_image_collection.metadata.enabled")
}
