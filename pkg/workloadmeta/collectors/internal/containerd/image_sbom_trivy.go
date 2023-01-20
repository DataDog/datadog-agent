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
)

// scan buffer needs to be very large as we cannot block containerd collector
const (
	imagesToScanBufferSize = 5000
)

func (c *collector) startSBOMCollection() error {
	if !sbomCollectionIsEnabled() {
		return nil
	}

	var err error
	enabledAnalyzers := config.Datadog.GetStringSlice("container_image_collection.sbom.analyzers")
	trivyConfiguration, err := trivy.DefaultCollectorConfig(enabledAnalyzers)
	if err != nil {
		return fmt.Errorf("error initializing trivy client: %w", err)
	}

	trivyConfiguration.ContainerdAccessor = func() (cutil.ContainerdItf, error) {
		return c.containerdClient, nil
	}
	c.trivyClient, err = trivy.NewCollector(trivyConfiguration)
	if err != nil {
		return fmt.Errorf("error initializing trivy client: %w", err)
	}

	c.imagesToScan = make(chan namespacedImage, imagesToScanBufferSize)

	go func() {
		for imageToScan := range c.imagesToScan {
			scanContext, cancel := context.WithTimeout(context.Background(), scanningTimeout())
			if err := c.extractBOMWithTrivy(scanContext, imageToScan); err != nil {
				log.Warnf("error extracting SBOM for image: namespace=%s name=%s, err: %s", imageToScan.namespace, imageToScan.image.Name(), err)
			}
			cancel()
		}
	}()

	return nil
}

func (c *collector) extractBOMWithTrivy(ctx context.Context, imageToScan namespacedImage) error {
	storedImage, err := c.store.GetImage(imageToScan.imageID)
	if err != nil {
		log.Infof("Image: %s/%s (id %s) not found in Workloadmeta, skipping scan", imageToScan.namespace, imageToScan.image.Name(), imageToScan.imageID)
		return nil
	}

	if storedImage.CycloneDXBOM != nil {
		// BOM already stored. Can happen when the same image ID is referenced
		// with different names.
		log.Debugf("Image: %s/%s (id %s) SBOM already available", imageToScan.namespace, imageToScan.image.Name(), imageToScan.imageID)
		return nil
	}

	scanFunc := c.trivyClient.ScanContainerdImage
	if config.Datadog.GetBool("workloadmeta.image_metadata_collection.collect_sboms_use_mount") {
		scanFunc = c.trivyClient.ScanContainerdImageFromFilesystem
	}

	bom, err := scanFunc(ctx, storedImage, imageToScan.image)
	if err != nil {
		return err
	}

	time.Sleep(timeBetweenScans())

	// Updating workloadmeta entities directly is not thread-safe, that's why we
	// generate an update event here instead.
	return c.handleImageCreateOrUpdate(ctx, imageToScan.namespace, storedImage.Name, bom)
}

func scanningTimeout() time.Duration {
	return time.Duration(config.Datadog.GetInt("workloadmeta.image_metadata_collection.collect_sboms_scan_timeout")) * time.Second
}

func timeBetweenScans() time.Duration {
	return time.Duration(config.Datadog.GetInt("workloadmeta.image_metadata_collection.collect_sboms_scan_interval")) * time.Second
}
