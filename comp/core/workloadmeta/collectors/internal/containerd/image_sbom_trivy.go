// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy

package containerd

import (
	"context"
	"fmt"
	"time"

	"github.com/CycloneDX/cyclonedx-go"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/containerd"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	scheduler "github.com/DataDog/datadog-agent/pkg/util/delayed_scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	schedulerBufferSize = 10
	// minBackoffFactor is set to 2 to avoid overlaps between retry intervals
	minBackoffFactor = 2
	// baseBackoffTime is set to 5 min to retry after 10/20/40... min
	baseBackoffTime = 5 * 60
	// maxBackoffTime is set to 1 hour to retry after 1h at most
	maxBackoffTime = 60 * 60
)

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && config.Datadog.GetBool("sbom.container_image.enabled")
}

func (c *collector) startSBOMCollection(ctx context.Context) error {
	if !sbomCollectionIsEnabled() {
		return nil
	}

	c.scanOptions = sbom.ScanOptionsFromConfig(config.Datadog, true)
	c.sbomScanner = scanner.GetGlobalScanner()
	c.scheduler = scheduler.NewScheduler(schedulerBufferSize)
	c.retryCountPerImage = make(map[string]retryInfo)
	c.backoffPolicy = backoff.NewExpBackoffPolicy(minBackoffFactor, baseBackoffTime, maxBackoffTime, 10, true)
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
	resultChan := make(chan sbom.ScanResult, 2000)
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

		switch image.SBOM.Status {
		case workloadmeta.Success:
			delete(c.retryCountPerImage, image.ID)
			log.Debugf("Image: %s/%s (id %s) SBOM already available", image.Namespace, image.Name, image.ID)
		case workloadmeta.Failed:
			c.retrySBOMGeneration(ctx, image, resultChan)
		case workloadmeta.Pending:
			c.sendScanRequest(ctx, image, resultChan)
		}
	}
}

func (c *collector) sendScanRequest(ctx context.Context, img *workloadmeta.ContainerImageMetadata, resultChan chan<- sbom.ScanResult) {
	if err := c.extractSBOMWithTrivy(ctx, img, resultChan); err != nil {
		log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", img.Namespace, img.Name, err)
	}
}

// retrySBOMGeneration retries SBOM generation for an image using an exponential backoff retry policy
func (c *collector) retrySBOMGeneration(ctx context.Context, storedImage *workloadmeta.ContainerImageMetadata, resultChan chan<- sbom.ScanResult) {
	if c.backoffPolicy == nil {
		log.Errorf("Backoff policy not initialized for SBOM collector, cannot retry failed scans")
		return
	}
	if c.scheduler == nil {
		log.Errorf("Scheduler not initialized for SBOM collector, cannot retry failed scans")
		return
	}
	if c.retryCountPerImage == nil {
		log.Errorf("Retry count map not initialized for SBOM collector, cannot retry failed scans")
		return
	}
	// For images with many repotags/repodigests, we need to retry once but retry will be
	// called several times (1 per container report)
	if c.retryCountPerImage[storedImage.ID].nextRetry.After(time.Now()) {
		log.Tracef("Image: %s/%s (id %s) SBOM generation failed, retry already scheduled", storedImage.Namespace, storedImage.Name, storedImage.ID)
		return
	}
	newErrCount := c.retryCountPerImage[storedImage.ID].errCount + 1
	nextTry := time.Now().Add(c.backoffPolicy.GetBackoffDuration(newErrCount))
	c.retryCountPerImage[storedImage.ID] = retryInfo{
		errCount:  newErrCount,
		nextRetry: nextTry,
	}
	log.Debugf("Image: %s/%s (id %s) SBOM generation failed, retrying at %s", storedImage.Namespace, storedImage.Name, storedImage.ID, nextTry.Format(time.RFC3339))
	c.scheduler.Schedule(
		func() { c.sendScanRequest(ctx, storedImage, resultChan) },
		nextTry,
	)
}

// extractSBOMWithTrivy emits a scan request to the SBOM scanner. The scan result will be sent to the resultChan.
func (c *collector) extractSBOMWithTrivy(_ context.Context, storedImage *workloadmeta.ContainerImageMetadata, resultChan chan<- sbom.ScanResult) error {
	containerdImage, err := c.containerdClient.Image(storedImage.Namespace, storedImage.Name)
	if err != nil {
		return err
	}

	scanRequest := &containerd.ScanRequest{
		Image:            containerdImage,
		ImageMeta:        storedImage,
		ContainerdClient: c.containerdClient,
		FromFilesystem:   config.Datadog.GetBool("sbom.container_image.use_mount"),
	}
	if err = c.sbomScanner.Scan(scanRequest, c.scanOptions, resultChan); err != nil {
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
