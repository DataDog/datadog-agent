// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && trivy

package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/CycloneDX/cyclonedx-go"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/docker"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	dutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func imageMetadataCollectionIsEnabled() bool {
	return config.Datadog.GetBool("container_image.enabled")
}

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

	scanner := collectors.GetDockerScanner()
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
			// We don't want to keep scanning if image channel is not empty but context is expired
			case <-ctx.Done():
				return

			case eventBundle, ok := <-imgEventsCh:
				if !ok {
					return
				}
				eventBundle.Acknowledge()

				for _, event := range eventBundle.Events {
					image := event.Entity.(*workloadmeta.ContainerImageMetadata)

					if image.SBOM.Status != workloadmeta.Pending {
						// BOM already stored. Can happen when the same image ID
						// is referenced with different names.
						log.Debugf("Image: %s/%s (id %s) SBOM already available", image.Namespace, image.Name, image.ID)
						continue
					}

					if err := c.extractSBOMWithTrivy(ctx, image.ID); err != nil {
						log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
					}
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case result, ok := <-resultChan:
				if !ok {
					return
				}
				if result.ImgMeta == nil {
					log.Errorf("Scan result does not hold the image identifier. Error: %s", result.Error)
					continue
				}
				status := workloadmeta.Success
				reportedError := ""
				var report *cyclonedx.BOM
				if result.Error != nil {
					// TODO: add a retry mechanism for retryable errors
					log.Errorf("Failed to generate SBOM for docker: %s", result.Error)
					status = workloadmeta.Failed
					reportedError = result.Error.Error()
				} else {
					bom, err := result.Report.ToCycloneDX()
					if err != nil {
						log.Errorf("Failed to extract SBOM from report")
						status = workloadmeta.Failed
						reportedError = result.Error.Error()
					}
					report = bom
				}

				sbom := &workloadmeta.SBOM{
					CycloneDXBOM:       report,
					GenerationTime:     result.CreatedAt,
					GenerationDuration: result.Duration,
					Status:             status,
					Error:              reportedError,
				}
				// Updating workloadmeta entities directly is not thread-safe, that's why we
				// generate an update event here instead.
				event := &dutil.ImageEvent{
					ImageID:   result.ImgMeta.ID,
					Action:    imageEventActionSbom,
					Timestamp: time.Now(),
				}
				if err := c.handleImageEvent(ctx, event, sbom); err != nil {
					log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", result.ImgMeta.Namespace, result.ImgMeta.Name, err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (c *collector) extractSBOMWithTrivy(_ context.Context, imageID string) error {
	scanRequest := docker.NewScanRequest(imageID)

	if err := c.sbomScanner.Scan(scanRequest); err != nil {
		log.Errorf("Failed to trigger SBOM generation for docker: %s", err)
		return err
	}

	return nil
}
