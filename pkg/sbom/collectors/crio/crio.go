// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio && trivy

package crio

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	crioUtil "github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

const resultChanSize = 1000

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	imageID string
}

// NewScanRequest creates a new scan request
func NewScanRequest(imageID string) sbom.ScanRequest {
	return scanRequest{imageID: imageID}
}

// Collector returns the collector name for the scan request
func (r scanRequest) Collector() string {
	return collectors.CrioCollector
}

// Type returns the scan request type based on ScanOptions
func (r scanRequest) Type(_ sbom.ScanOptions) string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.imageID
}

// Collector defines a CRI-O SBOM collector
type Collector struct {
	trivyCollector *trivy.Collector
	resChan        chan sbom.ScanResult
	opts           sbom.ScanOptions
	crioClient     crioUtil.Client
	wmeta          option.Option[workloadmeta.Component]

	closed bool
}

// CleanCache cleans the cache in the trivy collector
func (c *Collector) CleanCache() error {
	return c.trivyCollector.CleanCache()
}

// Init initializes the collector with configuration and workloadmeta component
func (c *Collector) Init(cfg config.Component, wmeta option.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.wmeta = wmeta
	c.trivyCollector = trivyCollector
	c.opts = sbom.ScanOptionsFromConfigForContainers(cfg)
	return nil
}

// Scan performs the scan using CRI-O methods
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	if !c.opts.OverlayFsScan {
		return sbom.ScanResult{Error: errors.New("overlayfs direct scan is not enabled, but required to scan CRI-O images")}
	}

	imageID := request.ID()

	if c.crioClient == nil {
		cl, err := crioUtil.NewCRIOClient()
		if err != nil {
			return sbom.ScanResult{Error: fmt.Errorf("error creating CRI-O client: %w", err)}
		}
		c.crioClient = cl
	}

	wmeta, ok := c.wmeta.Get()
	if !ok {
		return sbom.ScanResult{Error: errors.New("workloadmeta store is not initialized")}
	}

	imageMeta, err := wmeta.GetImage(imageID)
	if err != nil {
		return sbom.ScanResult{Error: fmt.Errorf("image metadata not found for image ID %s: %w", imageID, err)}
	}

	scanner := c.trivyCollector.ScanCRIOImageFromOverlayFS
	report, err := scanner(ctx, imageMeta, c.crioClient, c.opts)

	scanResult := sbom.ScanResult{
		Error:            err,
		Report:           report,
		ImgMeta:          imageMeta,
		GenerationMethod: "overlayfs",
	}

	return scanResult
}

// Type returns the container image scan type
func (c *Collector) Type() collectors.ScanType {
	return collectors.ContainerImageScanType
}

// Channel returns the channel to send scan results
func (c *Collector) Channel() chan sbom.ScanResult {
	return c.resChan
}

// Options returns the collector options
func (c *Collector) Options() sbom.ScanOptions {
	return c.opts
}

// Shutdown shuts down the collector
func (c *Collector) Shutdown() {
	if c.resChan != nil && !c.closed {
		close(c.resChan)
	}
	c.closed = true
}

func init() {
	collectors.RegisterCollector(collectors.CrioCollector, &Collector{
		resChan: make(chan sbom.ScanResult, resultChanSize),
	})
}
