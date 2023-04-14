// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy
// +build containerd,trivy

package containerd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/containerd"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && config.Datadog.GetBool("container_image_collection.sbom.enabled")
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

					if err := c.extractBOMWithTrivy(ctx, image); err != nil {
						log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
					}
				}
			}
		}
	}()

<<<<<<< HEAD
	go func() {
		defer func() {
			err := c.trivyClient.Close()
			if err != nil {
				log.Warnf("Unable to close trivy client: %v", err)
			}
		}()
		cleanTicker := time.NewTicker(config.Datadog.GetDuration("container_image_collection.sbom.cache_clean_interval"))
		for {
			select {
			case image := <-imagesToScanCh:
				scanContext, cancel := context.WithTimeout(ctx, scanningTimeout())
				if err := c.extractBOMWithTrivy(scanContext, image); err != nil {
					log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
				}
				cancel()

				time.Sleep(timeBetweenScans())
			case _ = <-cleanTicker.C:
				// the cache can not be cleaned during a scan
				if err := c.trivyClient.GetCacheCleaner().Clean(); err != nil {
					log.Warnf("Error cleaning SBOM cache: %v", err)
				}
			}
		}
	}()

=======
>>>>>>> main
	return nil
}

func (c *collector) extractBOMWithTrivy(ctx context.Context, storedImage *workloadmeta.ContainerImageMetadata) error {
	containerdImage, err := c.containerdClient.Image(storedImage.Namespace, storedImage.Name)
	if err != nil {
		return err
	}

	scanRequest := &containerd.ScanRequest{
		Image:          containerdImage,
		ImageMeta:      storedImage,
		FromFilesystem: config.Datadog.GetBool("container_image_collection.sbom.use_mount"),
	}

	ch := make(chan sbom.ScanResult, 1)
	if err = c.sbomScanner.Scan(scanRequest, c.scanOptions, ch); err != nil {
		return err
	}

	go func() {
		select {
		case <-ctx.Done():
		case result := <-ch:
			bom, err := result.Report.ToCycloneDX()
			if err != nil {
				log.Errorf("Failed to extract SBOM from report")
				return
			}

			sbom := workloadmeta.SBOM{
				CycloneDXBOM:       bom,
				GenerationTime:     result.CreatedAt,
				GenerationDuration: result.Duration,
			}

			// Updating workloadmeta entities directly is not thread-safe, that's why we
			// generate an update event here instead.
			if err := c.handleImageCreateOrUpdate(ctx, storedImage.Namespace, storedImage.Name, &sbom); err != nil {
				log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", storedImage.Namespace, storedImage.Name, err)
			}
		}
	}()

	return nil
}
