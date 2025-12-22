// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio && trivy

package crio

import (
	"context"
	"errors"
	"fmt"
	"os"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/crio"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	crioutil "github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// startSBOMCollection starts the SBOM collection process and subscribes to image metadata events.
func (c *collector) startSBOMCollection(ctx context.Context) error {
	if !sbomCollectionIsEnabled() {
		return nil
	}
	if err := overlayDirectoryAccess(); err != nil {
		return fmt.Errorf("SBOM collection enabled, but error accessing overlay directories: %w", err)
	}
	c.sbomScanner = scanner.GetGlobalScanner()
	if c.sbomScanner == nil {
		return errors.New("global SBOM scanner not found")
	}

	filter := workloadmeta.NewFilterBuilder().
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()

	imgEventsCh := c.store.Subscribe("SBOM collector", workloadmeta.NormalPriority, filter)

	scanner := collectors.GetCrioScanner()
	if scanner == nil {
		return errors.New("failed to retrieve CRI-O SBOM scanner")
	}

	resultChan := scanner.Channel()
	if resultChan == nil {
		return errors.New("failed to retrieve scanner result channel")
	}

	errs := c.sbomFilter.GetErrors()
	if len(errs) > 0 {
		return fmt.Errorf("failed to create container filter: %w", errors.Join(errs...))
	}

	go c.handleImageEvents(ctx, imgEventsCh, c.sbomFilter)
	go c.startScanResultHandler(ctx, resultChan)
	return nil
}

// handleImageEvents listens for container image metadata events, triggering SBOM generation for new images.
func (c *collector) handleImageEvents(ctx context.Context, imgEventsCh <-chan workloadmeta.EventBundle, filter workloadfilter.FilterBundle) {
	for {
		select {
		case <-ctx.Done():
			return
		case eventBundle, ok := <-imgEventsCh:
			if !ok {
				log.Warnf("Event channel closed, exiting event handling loop.")
				return
			}
			c.handleEventBundle(eventBundle, filter)
		}
	}
}

// handleEventBundle handles ContainerImageMetadata set events for which no SBOM generation attempt was done.
func (c *collector) handleEventBundle(eventBundle workloadmeta.EventBundle, containerImageFilter workloadfilter.FilterBundle) {
	eventBundle.Acknowledge()
	for _, event := range eventBundle.Events {
		image := event.Entity.(*workloadmeta.ContainerImageMetadata)

		filterableContainer := workloadfilter.CreateContainerImage(image.Name)
		if containerImageFilter != nil && containerImageFilter.IsExcluded(filterableContainer) {
			continue
		}

		if image.SBOM != nil && image.SBOM.Status != workloadmeta.Pending {
			continue
		}
		if err := c.extractSBOMWithTrivy(image.ID); err != nil {
			log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
		}
	}
}

// extractSBOMWithTrivy emits a scan request to the SBOM scanner. The scan result will be sent to the resultChan.
func (c *collector) extractSBOMWithTrivy(imageID string) error {
	if err := c.sbomScanner.Scan(crio.NewScanRequest(imageID)); err != nil {
		return fmt.Errorf("failed to trigger SBOM generation for CRI-O image ID %s: %w", imageID, err)
	}
	return nil
}

// startScanResultHandler receives SBOM scan results and updates the workloadmeta entities accordingly.
func (c *collector) startScanResultHandler(ctx context.Context, resultChan <-chan sbom.ScanResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-resultChan:
			if !ok {
				return
			}
			c.processScanResult(result)
		}
	}
}

// processScanResult updates the workloadmeta store with the SBOM for the image.
func (c *collector) processScanResult(result sbom.ScanResult) {
	if result.ImgMeta == nil {
		log.Errorf("Scan result missing image identifier. Error: %v", result.Error)
		return
	}

	sbom := result.ConvertScanResultToSBOM()
	csbom, err := sbomutil.CompressSBOM(sbom)
	if err != nil {
		log.Errorf("Failed to compress SBOM for image %s: %v", result.ImgMeta.ID, err)
		return
	}

	c.notifyStoreWithSBOMForImage(result.ImgMeta.ID, csbom)
}

// notifyStoreWithSBOMForImage notifies the store about the SBOM for a given image.
func (c *collector) notifyStoreWithSBOMForImage(imageID string, sbom *workloadmeta.CompressedSBOM) {
	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceTrivy,
			Entity: &workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   imageID,
				},
				SBOM: sbom,
			},
		},
	})
}

// overlayDirectoryAccess checks if the overlay directory and overlay-layers directory are accessible.
func overlayDirectoryAccess() error {
	overlayPath := crioutil.GetOverlayPath()
	if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
		return fmt.Errorf("overlay directory %s does not exist. Ensure this directory is mounted for SBOM collection to work", overlayPath)
	} else if err != nil {
		return fmt.Errorf("failed to check overlay directory %s: %w. Ensure this directory is mounted for SBOM collection to work", overlayPath, err)
	}

	overlayLayersPath := crioutil.GetOverlayLayersPath()
	if _, err := os.Stat(overlayLayersPath); os.IsNotExist(err) {
		return fmt.Errorf("overlay-layers directory %s does not exist. Ensure this directory is mounted for SBOM collection to work", overlayLayersPath)
	} else if err != nil {
		return fmt.Errorf("failed to check overlay-layers directory %s: %w. Ensure this directory is mounted for SBOM collection to work", overlayLayersPath, err)
	}

	return nil
}
