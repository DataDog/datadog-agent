// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package host

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// ScanRequest defines a scan request
// This struct should be hashable
type ScanRequest struct {
	Path string
}

// Collector returns the collector name
func (r ScanRequest) Collector() string {
	return collectors.HostCollector
}

// Type returns the scan request type
func (r ScanRequest) Type() string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r ScanRequest) ID() string {
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

// initOptions initializes the options of the collector
func (c *Collector) initOptions() {
	if flavor.GetFlavor() == flavor.SecurityAgent {
		c.opts = sbom.ScanOptions{Analyzers: []string{trivy.OSAnalyzers}, Fast: true}
	} else {
		c.opts = sbom.ScanOptionsFromConfig(config.Datadog, false)
	}
}

// Init initialize the host collector
func (c *Collector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	c.initOptions()
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	hostScanRequest, ok := request.(ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectors.HostCollector)}
	}
	log.Infof("host scan request [%v]", hostScanRequest.ID())

	report, err := c.trivyCollector.ScanFilesystem(ctx, hostScanRequest.Path, c.opts)
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
		resChan: make(chan sbom.ScanResult, 1),
	})
}
