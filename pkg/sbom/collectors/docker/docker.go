// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && trivy

package docker

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"

	"github.com/docker/docker/client"
)

// resultChanSize defines the result channel size
// 1000 is already a very large default value
const resultChanSize = 1000

type scannerFunc func(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client client.ImageAPIClient, scanOptions sbom.ScanOptions) (sbom.Report, error)

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	imageID string
}

// NewScanRequest creates a new scan request
func NewScanRequest(imageID string) sbom.ScanRequest {
	return scanRequest{imageID: imageID}
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return collectors.DockerCollector
}

// Type returns the scan request type
func (r scanRequest) Type(sbom.ScanOptions) string {
	return sbom.ScanDaemonType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.imageID
}

// Collector defines a collector
type Collector struct {
	trivyCollector *trivy.Collector
	resChan        chan sbom.ScanResult
	opts           sbom.ScanOptions
	cl             client.ImageAPIClient
	wmeta          optional.Option[workloadmeta.Component]

	closed bool
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return c.trivyCollector.CleanCache()
}

// Init initializes the collector
func (c *Collector) Init(cfg config.Component, wmeta optional.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.wmeta = wmeta
	c.trivyCollector = trivyCollector
	c.opts = sbom.ScanOptionsFromConfig(cfg, true)
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	dockerScanRequest, ok := request.(scanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectors.DockerCollector)}
	}

	if c.cl == nil {
		cl, err := docker.GetDockerUtil()
		if err != nil {
			return sbom.ScanResult{Error: fmt.Errorf("error creating docker client: %s", err)}
		}
		c.cl = cl.RawClient()
	}

	wmeta, ok := c.wmeta.Get()
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("workloadmeta store is not initialized")}
	}

	imageMeta, err := wmeta.GetImage(dockerScanRequest.ID())
	if err != nil {
		return sbom.ScanResult{Error: fmt.Errorf("image metadata not found for image id %s: %s", dockerScanRequest.ID(), err)}
	}

	var scanner scannerFunc
	if c.opts.OverlayFsScan {
		scanner = c.trivyCollector.ScanDockerImageFromGraphDriver
	} else {
		scanner = c.trivyCollector.ScanDockerImage
	}
	report, err := scanner(
		ctx,
		imageMeta,
		c.cl,
		c.opts,
	)

	return sbom.ScanResult{
		Error:   err,
		Report:  report,
		ImgMeta: imageMeta,
	}
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
	collectors.RegisterCollector(collectors.DockerCollector, &Collector{
		resChan: make(chan sbom.ScanResult, resultChanSize),
	})
}
