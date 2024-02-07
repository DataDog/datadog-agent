// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy

package containerd

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/containerd"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && config.Datadog.GetBool("sbom.container_image.enabled")
}

func (c *collector) startSBOMCollection(ctx context.Context) error {
	if !sbomCollectionIsEnabled() {
		return nil
	}

	c.sbomScanner = scanner.GetGlobalScanner()
	if c.sbomScanner == nil {
		return fmt.Errorf("error retrieving global SBOM scanner")
	}

	filterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindContainerImageMetadata},
		Source:    workloadmeta.SourceAll,
		EventType: workloadmeta.EventTypeSet,
	}
	imgEventsCh := c.store.Subscribe(
		"SBOM collector",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&filterParams),
	)
	scanner := collectors.GetContainerdScanner()
	if scanner == nil {
		return fmt.Errorf("error retrieving global docker scanner")
	}
	resultChan := scanner.Channel()
	if resultChan == nil {
		return fmt.Errorf("error retrieving global docker scanner channel")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case eventBundle, ok := <-imgEventsCh:
				if !ok {
					// closed channel case
					return
				}
				c.handleEventBundle(ctx, eventBundle, resultChan)
			}
		}
	}()

	go c.startScanResultHandler(ctx, resultChan)

	return nil
}

// handleEventBundle handles ContainerImageMetadata set events for which no SBOM generation attempt was done.
func (c *collector) handleEventBundle(ctx context.Context, eventBundle workloadmeta.EventBundle, resultChan chan<- sbom.ScanResult) {
	eventBundle.Acknowledge()
	for _, event := range eventBundle.Events {
		image := event.Entity.(*workloadmeta.ContainerImageMetadata)

		if image.SBOM.Status != workloadmeta.Pending {
			// A generation attempt has already been done. In that case, it should go through the retry logic.
			// We can't handle them here otherwise it would keep retrying in a while loop.
			log.Debugf("Image: %s/%s (id %s) SBOM already available", image.Namespace, image.Name, image.ID)
			continue
		}

		if err := c.extractSBOMWithTrivy(ctx, image.ID); err != nil {
			log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
		}
	}
}

// extractSBOMWithTrivy emits a scan request to the SBOM scanner. The scan result will be sent to the resultChan.
func (c *collector) extractSBOMWithTrivy(_ context.Context, imageID string) error {
	scanRequest := containerd.ScanRequest{
		ImageID: imageID,
	}
	if err := c.sbomScanner.Scan(scanRequest); err != nil {
		log.Errorf("Failed to trigger SBOM generation for containerd: %s", err)
		return err
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
			c.processScanResult(ctx, result)
		}
	}
}

func (c *collector) processScanResult(ctx context.Context, result sbom.ScanResult) {
	if result.ImgMeta == nil {
		log.Errorf("Scan result does not hold the image identifier. Error: %s", result.Error)
		return
	}

	// Updating workloadmeta entities directly is not thread-safe, that's why we
	// generate an update event here instead.
	if err := c.handleImageCreateOrUpdate(ctx, result.ImgMeta.Namespace, result.ImgMeta.Name, convertScanResultToSBOM(result)); err != nil {
		log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", result.ImgMeta.Namespace, result.ImgMeta.Name, err)
	}
}

func convertScanResultToSBOM(result sbom.ScanResult) *workloadmeta.SBOM {
	status := workloadmeta.Success
	reportedError := ""
	var report *cyclonedx.BOM

	if result.Error != nil {
		log.Errorf("Failed to generate SBOM for containerd image: %s", result.Error)
		status = workloadmeta.Failed
		reportedError = result.Error.Error()
	} else if bom, err := result.Report.ToCycloneDX(); err != nil {
		log.Errorf("Failed to extract SBOM from report")
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
