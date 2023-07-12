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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/docker"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	dutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
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

	c.scanOptions = sbom.ScanOptionsFromConfig(config.Datadog, true)
	c.sbomScanner = scanner.GetGlobalScanner()
	if c.sbomScanner == nil {
		return fmt.Errorf("error retrieving global SBOM scanner")
	}

	imgEventsCh := c.store.Subscribe(
		"SBOM collector",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{workloadmeta.KindContainerImageMetadata},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeSet,
		),
	)

	resultChan := make(chan sbom.ScanResult, 2000)
	go func() {
		for {
			select {
			// We don't want to keep scanning if image channel is not empty but context is expired
			case <-ctx.Done():
				return

			case eventBundle := <-imgEventsCh:
				close(eventBundle.Ch)

				for _, event := range eventBundle.Events {
					image := event.Entity.(*workloadmeta.ContainerImageMetadata)

					if image.SBOM != nil {
						// BOM already stored. Can happen when the same image ID
						// is referenced with different names.
						log.Debugf("Image: %s/%s (id %s) SBOM already available", image.Namespace, image.Name, image.ID)
						continue
					}

					if err := c.extractSBOMWithTrivy(ctx, image, resultChan); err != nil {
						log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
					}
				}
			}
		}
	}()

	go func() {
		for result := range resultChan {
			if result.Error != nil {
				// TODO: add a retry mechanism for retryable errors
				log.Errorf("Failed to generate SBOM for docker: %s", result.Error)
				continue
			}

			bom, err := result.Report.ToCycloneDX()
			if err != nil {
				log.Errorf("Failed to extract SBOM from report")
				continue
			}

			sbom := &workloadmeta.SBOM{
				CycloneDXBOM:       bom,
				GenerationTime:     result.CreatedAt,
				GenerationDuration: result.Duration,
			}

			// Updating workloadmeta entities directly is not thread-safe, that's why we
			// generate an update event here instead.
			event := &dutil.ImageEvent{
				ImageID:   result.ImgMeta.ID,
				Action:    dutil.ImageEventActionSbom,
				Timestamp: time.Now(),
			}
			if err := c.handleImageEvent(ctx, event, sbom); err != nil {
				log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", result.ImgMeta.Namespace, result.ImgMeta.Name, err)
			}
		}

	}()

	return nil
}

func (c *collector) extractSBOMWithTrivy(ctx context.Context, storedImage *workloadmeta.ContainerImageMetadata, resultChan chan<- sbom.ScanResult) error {
	scanRequest := &docker.ScanRequest{
		ImageMeta:    storedImage,
		DockerClient: c.dockerUtil.RawClient(),
	}

	if err := c.sbomScanner.Scan(scanRequest, c.scanOptions, resultChan); err != nil {
		log.Errorf("Failed to trigger SBOM generation for docker: %s", err)
		return err
	}

	return nil
}
