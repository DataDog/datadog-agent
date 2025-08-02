// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

// Package crio implements the crio Workloadmeta collector.
package crio

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "crio"
	componentName = "workloadmeta-crio"
)

type collector struct {
	id             string
	client         crio.Client
	store          workloadmeta.Component
	catalog        workloadmeta.AgentType
	seenContainers map[workloadmeta.EntityID]struct{}
	seenImages     map[workloadmeta.EntityID]struct{}
	sbomScanner    *scanner.Scanner //nolint: unused
}

// NewCollector initializes a new CRI-O collector.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:             collectorID,
			seenContainers: make(map[workloadmeta.EntityID]struct{}),
			seenImages:     make(map[workloadmeta.EntityID]struct{}),
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
		return fmt.Errorf("CRI-O client creation failed: %v", err)
	}
	c.client = client

	if err := c.startSBOMCollection(ctx); err != nil {
		return fmt.Errorf("SBOM collection initialization failed: %v", err)
	}

	if imageMetadataCollectionIsEnabled() {
		if err := checkOverlayImageDirectoryExists(); err != nil {
			log.Warnf("Overlay image directory check failed: %v", err)
		}
	}

	return nil
}

// Pull gathers container data.
func (c *collector) Pull(ctx context.Context) error {
	containers, err := c.client.GetAllContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to pull container list: %v", err)
	}

	seenContainers := make(map[workloadmeta.EntityID]struct{})
	seenImages := make(map[workloadmeta.EntityID]struct{})
	containerEvents := make([]workloadmeta.CollectorEvent, 0, len(containers))

	collectImages := imageMetadataCollectionIsEnabled()

	for _, container := range containers {
		// Generate container event
		containerEvent := c.convertContainerToEvent(ctx, container)
		seenContainers[containerEvent.Entity.GetID()] = struct{}{}
		containerEvents = append(containerEvents, containerEvent)
	}

	// Handle image collection using the optimized approach
	if collectImages {
		// Use the new optimized method to get image events
		imageEvents, err := c.generateImageEventsFromImageList(ctx)
		if err != nil {
			log.Warnf("Optimized approach failed: %v - falling back to per-container approach", err)
			// Fall back to the old per-container approach if image list fails
			imageEvents = make([]workloadmeta.CollectorEvent, 0, len(containers))
			for _, container := range containers {
				imageEvent, err := c.generateImageEventFromContainer(ctx, container)
				if err != nil {
					log.Warnf("Image event generation failed for container %+v: %v", container, err)
					continue
				}
				imageEvents = append(imageEvents, *imageEvent)
			}
		}

		// Build seenImages map from the events for cleanup
		for _, event := range imageEvents {
			if event.Type == workloadmeta.EventTypeSet {
				seenImages[event.Entity.GetID()] = struct{}{}
			}
		}

		// Handle unset events for images that are no longer present
		for seenID := range c.seenImages {
			if _, ok := seenImages[seenID]; !ok {
				unsetEvent := generateUnsetImageEvent(seenID)
				imageEvents = append(imageEvents, *unsetEvent)
			}
		}
		c.seenImages = seenImages
		c.store.Notify(imageEvents)
	}

	// Handle unset events for containers
	for seenID := range c.seenContainers {
		if _, ok := seenContainers[seenID]; !ok {
			unsetEvent := generateUnsetContainerEvent(seenID)
			containerEvents = append(containerEvents, unsetEvent)
		}
	}
	c.seenContainers = seenContainers
	c.store.Notify(containerEvents)

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

// imageMetadataCollectionIsEnabled checks if image metadata collection is enabled via configuration.
func imageMetadataCollectionIsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("container_image.enabled")
}

// sbomCollectionIsEnabled returns true if SBOM collection is enabled.
func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && pkgconfigsetup.Datadog().GetBool("sbom.container_image.enabled")
}

// checkOverlayImageDirectoryExists checks if the overlay-image directory exists.
func checkOverlayImageDirectoryExists() error {
	overlayImagePath := crio.GetOverlayImagePath()
	if _, err := os.Stat(overlayImagePath); os.IsNotExist(err) {
		return fmt.Errorf("overlay-image directory %s does not exist. Ensure this directory is mounted to enable access to layer size and media type", overlayImagePath)
	} else if err != nil {
		return fmt.Errorf("failed to check overlay-image directory %s: %w. Ensure this directory is mounted to enable access to layer size and media type", overlayImagePath, err)
	}
	return nil
}
