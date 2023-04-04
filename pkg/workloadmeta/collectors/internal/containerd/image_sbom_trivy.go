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
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

// scan buffer needs to be very large as we cannot block containerd collector
const (
	imagesToScanBufferSize = 5000
)

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && config.Datadog.GetBool("container_image_collection.sbom.enabled")
}

func (c *collector) startSBOMCollection(ctx context.Context) error {
	if !sbomCollectionIsEnabled() {
		return nil
	}

	var err error
	enabledAnalyzers := config.Datadog.GetStringSlice("container_image_collection.sbom.analyzers")
	trivyConfiguration := trivy.DefaultCollectorConfig(enabledAnalyzers, config.Datadog.GetString("container_image_collection.sbom.cache_directory"))
	trivyConfiguration.ClearCacheOnClose = config.Datadog.GetBool("container_image_collection.sbom.clear_cache_on_exit")
	trivyConfiguration.ContainerdAccessor = func() (cutil.ContainerdItf, error) {
		return c.containerdClient, nil
	}

	c.trivyClient, err = trivy.NewCollector(trivyConfiguration)
	if err != nil {
		return fmt.Errorf("error initializing trivy client: %w", err)
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

	imagesToScanCh := make(chan *workloadmeta.ContainerImageMetadata, imagesToScanBufferSize)

	go func() {
		for {
			select {
			// We don't want to keep scanning if image channel is not empty but context is expired
			case <-ctx.Done():
				close(imagesToScanCh)
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

					// Don't scan the image here. Enqueue it instead because we
					// need to keep reading events from workloadmeta to avoid
					// blocking it.

					imagesToScanCh <- image
				}
			}
		}
	}()

	go func() {
		defer func() {
			err := c.trivyClient.Close()
			if err != nil {
				log.Warnf("Unable to close trivy client: %v", err)
			}
		}()

		for image := range imagesToScanCh {
			scanContext, cancel := context.WithTimeout(ctx, scanningTimeout())
			if err := c.extractBOMWithTrivy(scanContext, image); err != nil {
				log.Warnf("Error extracting SBOM for image: namespace=%s name=%s, err: %s", image.Namespace, image.Name, err)
			}
			cancel()

			time.Sleep(timeBetweenScans())
		}
	}()

	return nil
}

func (c *collector) extractBOMWithTrivy(ctx context.Context, image *workloadmeta.ContainerImageMetadata) error {
	containerdImage, err := c.containerdClient.Image(image.Namespace, image.Name)
	if err != nil {
		return err
	}

	scanFunc := c.trivyClient.ScanContainerdImage
	if config.Datadog.GetBool("container_image_collection.sbom.use_mount") {
		scanFunc = c.trivyClient.ScanContainerdImageFromFilesystem
	}

	tStartScan := time.Now()
	report, err := scanFunc(ctx, image, containerdImage)
	if err != nil {
		return err
	}

	cycloneDXBOM, err := report.ToCycloneDX()
	if err != nil {
		return err
	}

	scanDuration := time.Since(tStartScan)

	telemetry.SBOMGenerationDuration.Observe(scanDuration.Seconds())

	sbom := workloadmeta.SBOM{
		CycloneDXBOM:       cycloneDXBOM,
		GenerationTime:     tStartScan,
		GenerationDuration: scanDuration,
	}

	// Updating workloadmeta entities directly is not thread-safe, that's why we
	// generate an update event here instead.
	return c.handleImageCreateOrUpdate(ctx, image.Namespace, image.Name, &sbom)
}

func scanningTimeout() time.Duration {
	return time.Duration(config.Datadog.GetInt("container_image_collection.sbom.scan_timeout")) * time.Second
}

func timeBetweenScans() time.Duration {
	return time.Duration(config.Datadog.GetInt("container_image_collection.sbom.scan_interval")) * time.Second
}
