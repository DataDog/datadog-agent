// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

// Package crio implements the crio Workloadmeta collector.
package crio

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"sync"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "crio"
	componentName = "workloadmeta-crio"
)

type collector struct {
	id              string
	client          crio.Client
	store           workloadmeta.Component
	catalog         workloadmeta.AgentType
	seenContainers  map[workloadmeta.EntityID]struct{}
	seenImages      map[workloadmeta.EntityID]struct{}
	handleImagesMut sync.Mutex
	sbomScanner     *scanner.Scanner //nolint: unused
}

// NewCollector initializes a new CRI-O collector.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:             collectorID,
			seenContainers: make(map[workloadmeta.EntityID]struct{}),
			catalog:        workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start initializes the collector for workloadmeta.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Crio) {
		return dderrors.NewDisabled(componentName, "Crio not detected")
	}
	c.store = store

	client, err := crio.NewCRIOClient()
	if err != nil {
		log.Errorf("CRI-O client creation failed: %v", err)
		client.Close()
		return err
	}
	c.client = client

	if err := c.startSBOMCollection(ctx); err != nil {
		log.Errorf("SBOM collection initialization failed: %v", err)
		return err
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	containers, err := c.client.GetAllContainers(ctx)
	if err != nil {
		log.Errorf("Failed to pull container list: %v", err)
		return err
	}

	// Lock image processing to prevent concurrent modifications
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()

	// Process container events and generate image events
	seenContainers := make(map[workloadmeta.EntityID]struct{})
	seenImages := make(map[string]*workloadmeta.CollectorEvent) // Map to store unique images by ID
	containerEvents := make([]workloadmeta.CollectorEvent, 0, len(containers))

	for _, container := range containers {
		// Generate container event
		containerEvent := c.convertContainerToEvent(ctx, container)
		seenContainers[containerEvent.Entity.GetID()] = struct{}{}
		containerEvents = append(containerEvents, containerEvent)

		// Generate associated image event from container's image reference
		if container.Image == nil || container.Image.Image == "" {
			log.Warnf("Skipped container with empty image reference: %+v", container)
			continue
		}

		// Fetch and convert image to event with namespace
		imageEvent := c.generateImageEventFromContainer(ctx, container)
		if imageEvent.Type == workloadmeta.EventTypeUnset {
			log.Warnf("Image event generation failed for container image ID: %s", container.Image.Image)
			continue
		}
		seenImages[imageEvent.Entity.GetID().ID] = &imageEvent // Store unique images by ID
	}

	// Handle unset events for containers
	for seenID := range c.seenContainers {
		if _, ok := seenContainers[seenID]; !ok {
			unsetEvent := generateUnsetContainerEvent(seenID)
			containerEvents = append(containerEvents, unsetEvent)
		}
	}

	// Handle unset events for images
	for seenID := range c.seenImages {
		if _, ok := seenImages[seenID.ID]; !ok {
			unsetEvent := generateUnsetImageEvent(seenID)
			seenImages[unsetEvent.Entity.GetID().ID] = &unsetEvent
		}
	}

	// Update seen maps and notify all events
	c.seenContainers = seenContainers
	c.seenImages = make(map[workloadmeta.EntityID]struct{})
	for id := range seenImages {
		c.seenImages[workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: id}] = struct{}{}
	}

	// Collect all events from the map and containerEvents list
	allEvents := containerEvents
	for _, event := range seenImages {
		allEvents = append(allEvents, *event)
	}
	c.store.Notify(allEvents)
	return nil
}

// GetID returns the collector ID.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog returns the workloadmeta agent type.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}
