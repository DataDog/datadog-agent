// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio && trivy

package crio

import (
	"context"
	"fmt"
	"os"

	"github.com/CycloneDX/cyclonedx-go"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/crio"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
		return fmt.Errorf("global SBOM scanner not found")
	}

	filter := workloadmeta.NewFilterBuilder().
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()

	imgEventsCh := c.store.Subscribe("SBOM collector", workloadmeta.NormalPriority, filter)

	scanner := collectors.GetCrioScanner()
	if scanner == nil {
		return fmt.Errorf("failed to retrieve CRI-O SBOM scanner")
	}

	resultChan := scanner.Channel()
	if resultChan == nil {
		return fmt.Errorf("failed to retrieve scanner result channel")
	}

	containerImageFilter, err := collectors.NewSBOMContainerFilter()
	if err != nil {
		return fmt.Errorf("failed to create container filter: %w", err)
	}

	go c.handleImageEvents(ctx, imgEventsCh, containerImageFilter)
	go c.startScanResultHandler(ctx, resultChan)
	return nil
}

// handleImageEvents listens for container image metadata events, triggering SBOM generation for new images.
func (c *collector) handleImageEvents(ctx context.Context, imgEventsCh <-chan workloadmeta.EventBundle, filter *containers.Filter) {
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
func (c *collector) handleEventBundle(eventBundle workloadmeta.EventBundle, containerImageFilter *containers.Filter) {
	eventBundle.Acknowledge()
	for _, event := range eventBundle.Events {
		image := event.Entity.(*workloadmeta.ContainerImageMetadata)

		if containerImageFilter != nil && containerImageFilter.IsExcluded(nil, "", image.Name, "") {
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

	c.notifyStoreWithSBOMForImage(result.ImgMeta.ID, convertScanResultToSBOM(result))
}

// convertScanResultToSBOM converts an SBOM scan result to a workloadmeta SBOM.
func convertScanResultToSBOM(result sbom.ScanResult) *workloadmeta.SBOM {
	status := workloadmeta.Success
	reportedError := ""
	var report *cyclonedx.BOM

	if result.Error != nil {
		log.Errorf("SBOM generation failed for image: %v", result.Error)
		status = workloadmeta.Failed
		reportedError = result.Error.Error()
	} else if bom, err := result.Report.ToCycloneDX(); err != nil {
		log.Errorf("Failed to convert report to CycloneDX BOM.")
		status = workloadmeta.Failed
		reportedError = err.Error()
	} else {
		report = bom
	}

	return &workloadmeta.SBOM{
		CycloneDXBOM:       report,
		GenerationTime:     result.CreatedAt,
		GenerationDuration: result.Duration,
		Status:             status,
		Error:              reportedError,
	}
}

// notifyStoreWithSBOMForImage notifies the store about the SBOM for a given image.
func (c *collector) notifyStoreWithSBOMForImage(imageID string, sbom *workloadmeta.SBOM) {
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
