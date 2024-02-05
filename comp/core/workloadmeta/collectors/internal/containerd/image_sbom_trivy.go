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
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/containerd"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// baseBackoffTime is set to 10 minutes for the retry mechanism
	baseBackoffTime = 10 * time.Minute
	// maxBackoffTime is set to 1 hour to retry after 1h at most
	maxBackoffTime = time.Hour
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
	c.queue = workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(baseBackoffTime, maxBackoffTime))
	if c.sbomScanner == nil {
		return fmt.Errorf("error retrieving global SBOM scanner")
	}

	filterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindContainerImageMetadata},
		Source:    workloadmeta.SourceAll,
		EventType: workloadmeta.EventTypeAll,
	}
	imgEventsCh := c.store.Subscribe(
		"SBOM collector",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&filterParams),
	)
	resultChan := make(chan sbom.ScanResult, 2000)

	// First start the result handlers before listening to events to avoid missing events
	go c.startScanResultHandler(ctx, resultChan)
	go c.startRetryLoop(ctx, resultChan)

	if c.queue == nil {
		return fmt.Errorf("error creating SBOM queue")
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				if c.queue != nil {
					c.queue.ShutDown()
				}
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

	return nil
}

// startRetryLoop starts a loop that will retry SBOM generation for images that failed to generate a SBOM
func (c *collector) startRetryLoop(ctx context.Context, resultChan chan<- sbom.ScanResult) {
	for {
		imgID, shutdown := c.queue.Get()
		if shutdown {
			log.Debugf("shutdown sbom retry queue")
			return
		}
		id, ok := imgID.(string)
		if !ok {
			log.Errorf("expected string, got %T", imgID)
			c.queue.Forget(imgID)
			c.queue.Done(imgID)
			continue
		}
		imgMeta, err := c.store.GetImage(id)
		if err != nil {
			log.Debugf("image %s not found in store", id)
			c.queue.Forget(imgID)
			c.queue.Done(imgID)
			continue
		}
		log.Debugf("retrying SBOM generation for image %s (id: %s)", imgMeta.Name, imgMeta.ID)
		c.sendScanRequest(ctx, imgMeta, resultChan)
		// c.queue.Done() cannot be called here as it as the sbom is being processed
	}
}

// handleUnsetEvent handles ContainerImageMetadata unset events.
// It removes the image from the retry queue.
func (c *collector) handleUnsetEvent(event workloadmeta.Event) {
	if c.queue != nil && event.Entity != nil {
		id := event.Entity.GetID()
		c.queue.Forget(id)
		c.queue.Done(id)
	}
}

// handleEventBundle handles ContainerImageMetadata set events for which no SBOM generation attempt was done.
func (c *collector) handleEventBundle(ctx context.Context, eventBundle workloadmeta.EventBundle, resultChan chan<- sbom.ScanResult) {
	eventBundle.Acknowledge()
	for _, event := range eventBundle.Events {
		if event.Type == workloadmeta.EventTypeUnset {
			c.handleUnsetEvent(event)
			continue
		}
		image := event.Entity.(*workloadmeta.ContainerImageMetadata)
		switch image.SBOM.Status {
		case workloadmeta.Success:
			log.Debugf("SBOM available for image: %s/%s (id %s) available", image.Namespace, image.Name, image.ID)
			// Forget and Done are safe to call even if the image is not in the retry queue.
			c.queue.Forget(image.ID)
			c.queue.Done(image.ID)
		case workloadmeta.Failed:
			// We can't store the image itself because it is updated at every iteration so the queue won't be able to find it.
			c.queue.AddRateLimited(image.ID)
			// Done needs to be called after processing the image. It is only processed once the scanner returns a result.
			c.queue.Done(image.ID)
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
