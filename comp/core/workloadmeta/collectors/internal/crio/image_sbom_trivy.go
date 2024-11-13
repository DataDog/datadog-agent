// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio && trivy

package crio

import (
	"context"
	"fmt"

	"github.com/CycloneDX/cyclonedx-go"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/crio"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && pkgconfigsetup.Datadog().GetBool("sbom.container_image.enabled")
}

func (c *collector) startSBOMCollection(ctx context.Context) error {
	if !sbomCollectionIsEnabled() {
		return nil
	}
	c.sbomScanner = scanner.GetGlobalScanner()
	if c.sbomScanner == nil {
		err := fmt.Errorf("global SBOM scanner not found")
		log.Errorf("%v", err)
		return err
	}

	filter := workloadmeta.NewFilterBuilder().
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()

	imgEventsCh := c.store.Subscribe("SBOM collector", workloadmeta.NormalPriority, filter)

	scanner := collectors.GetCrioScanner()
	if scanner == nil {
		err := fmt.Errorf("failed to retrieve CRI-O SBOM scanner")
		log.Errorf("%v", err)
		return err
	}

	resultChan := scanner.Channel()
	if resultChan == nil {
		err := fmt.Errorf("failed to retrieve scanner result channel")
		log.Errorf("%v", err)
		return err
	}

	go c.handleImageEvents(ctx, imgEventsCh)
	go c.startScanResultHandler(ctx, resultChan)
	return nil
}

// handleImageEvents listens for container image metadata events, triggering SBOM generation for new images.
func (c *collector) handleImageEvents(ctx context.Context, imgEventsCh <-chan workloadmeta.EventBundle) {
	for {
		select {
		case <-ctx.Done():
			return
		case eventBundle, ok := <-imgEventsCh:
			if !ok {
				log.Warnf("Event channel closed, exiting event handling loop.")
				return
			}
			c.handleEventBundle(eventBundle)
		}
	}
}

// handleEventBundle handles ContainerImageMetadata set events for which no SBOM generation attempt was done.
func (c *collector) handleEventBundle(eventBundle workloadmeta.EventBundle) {
	eventBundle.Acknowledge()
	for _, event := range eventBundle.Events {
		image := event.Entity.(*workloadmeta.ContainerImageMetadata)

		if image.SBOM.Status != workloadmeta.Pending {
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
		return fmt.Errorf("failed to trigger SBOM generation for CRI-O image ID %s: %v", imageID, err)
	}
	return nil
}

// The scanResultHandler receives SBOM scan results and updates the workloadmeta entities accordingly.
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

func (c *collector) processScanResult(result sbom.ScanResult) {
	if result.ImgMeta == nil {
		log.Errorf("Scan result missing image metadata. Error: %v", result.Error)
		return
	}

	if err := c.updateSBOMForImage(result.ImgMeta.ID, convertScanResultToSBOM(result)); err != nil {
		log.Warnf("Error updating SBOM for image: namespace=%s name=%s, err: %s", result.ImgMeta.Namespace, result.ImgMeta.Name, err)
	}
}

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
