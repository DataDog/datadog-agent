// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy

// Package containerd holds containerd related files
package containerd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// resultChanSize defines the result channel size
// 1000 is already a very large default value
const resultChanSize = 1000

// ScanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type ScanRequest struct {
	ImageID string
}

// Collector returns the collector name
func (r ScanRequest) Collector() string {
	return collectors.ContainerdCollector
}

// Type returns the scan request type
func (r ScanRequest) Type() string {
	if config.Datadog.GetBool("sbom.container_image.use_mount") {
		return sbom.ScanFilesystemType
	}
	return sbom.ScanDaemonType
}

// ID returns the scan request ID
func (r ScanRequest) ID() string {
	return r.ImageID
}

// Collector defines a containerd collector
type Collector struct {
	trivyCollector   *trivy.Collector
	resChan          chan sbom.ScanResult
	opts             sbom.ScanOptions
	containerdClient cutil.ContainerdItf
	wmeta            optional.Option[workloadmeta.Component]

	fromFileSystem bool
	closed         bool
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return c.trivyCollector.CleanCache()
}

// Init initializes the collector
func (c *Collector) Init(cfg config.Config, wmeta optional.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.wmeta = wmeta
	c.trivyCollector = trivyCollector
	c.fromFileSystem = cfg.GetBool("sbom.container_image.use_mount")
	c.opts = sbom.ScanOptionsFromConfig(config.Datadog, true)
	return nil
}

// Scan performs the scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	containerdScanRequest, ok := request.(ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectors.ContainerdCollector)}
	}

	if c.containerdClient == nil {
		cl, err := cutil.NewContainerdUtil()
		if err != nil {
			return sbom.ScanResult{Error: fmt.Errorf("error creating containerd client: %s", err)}
		}
		c.containerdClient = cl
	}

	wmeta, ok := c.wmeta.Get()
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("workloadmeta store is not initialized")}
	}
	imageMeta, err := wmeta.GetImage(containerdScanRequest.ID())
	if err != nil {
		return sbom.ScanResult{Error: fmt.Errorf("image metadata not found for image id %s: %s", containerdScanRequest.ID(), err)}
	}
	log.Infof("containerd scan request [%v]: scanning image %v", containerdScanRequest.ID(), imageMeta.Name)

	image, err := c.containerdClient.Image(imageMeta.Namespace, imageMeta.Name)
	if err != nil {
		return sbom.ScanResult{Error: fmt.Errorf("error getting image %s/%s: %s", imageMeta.Namespace, imageMeta.Name, err)}
	}

	var report sbom.Report
	if c.fromFileSystem {
		report, err = c.trivyCollector.ScanContainerdImageFromFilesystem(
			ctx,
			imageMeta,
			image,
			c.containerdClient,
			c.opts,
		)
	} else {
		report, err = c.trivyCollector.ScanContainerdImage(
			ctx,
			imageMeta,
			image,
			c.containerdClient,
			c.opts,
		)
	}
	scanResult := sbom.ScanResult{
		Error:   err,
		Report:  report,
		ImgMeta: imageMeta,
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
	collectors.RegisterCollector(collectors.ContainerdCollector, &Collector{
		resChan: make(chan sbom.ScanResult, resultChanSize),
	})
}
