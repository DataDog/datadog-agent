// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package host

import (
	"context"
	"fmt"
	"io/fs"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// channelSize defines the result channel size
// It doesn't need more than 1 because the host collector should
// not trigger multiple scans at the same time unlike for container-images.
const channelSize = 1

// scanRequest defines a scan request. This struct should be
// hashable to be pushed in the work queue for processing.
type scanRequest struct {
	Path string
	FS   fs.FS
}

// NewScanRequest creates a new scan request
func NewScanRequest(path string, fs fs.FS) sbom.ScanRequest {
	return scanRequest{Path: path, FS: fs}
}

// Collector returns the collector name
func (r scanRequest) Collector() string {
	return collectors.HostCollector
}

// Type returns the scan request type
func (r scanRequest) Type(sbom.ScanOptions) string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r scanRequest) ID() string {
	return r.Path
}

// Collector defines a host collector
type Collector struct {
	trivyCollector *trivy.Collector
	resChan        chan sbom.ScanResult
	opts           sbom.ScanOptions

	closed bool
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(cfg config.Component, wmeta optional.Option[workloadmeta.Component]) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	if flavor.GetFlavor() == flavor.SecurityAgent {
		c.opts = sbom.ScanOptions{Analyzers: []string{trivy.OSAnalyzers}, Fast: true, CollectFiles: true}
	} else {
		c.opts = sbom.ScanOptionsFromConfig(cfg, false)
	}
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	hostScanRequest, ok := request.(scanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectors.HostCollector)}
	}
	log.Infof("host scan request [%v]", hostScanRequest.ID())

	report, err := c.trivyCollector.ScanFilesystem(ctx, hostScanRequest.FS, hostScanRequest.Path, c.opts)
	return sbom.ScanResult{
		Error:  err,
		Report: report,
	}
}

// Type returns the container image scan type
func (c *Collector) Type() collectors.ScanType {
	return collectors.HostScanType
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
	collectors.RegisterCollector(collectors.HostCollector, &Collector{
		resChan: make(chan sbom.ScanResult, channelSize),
	})
}
