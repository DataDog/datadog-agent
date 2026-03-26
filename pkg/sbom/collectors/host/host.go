// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package host

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/aquasecurity/trivy/pkg/types"
)

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
func (c *Collector) Init(cfg config.Component, wmeta option.Option[workloadmeta.Component]) error {
	return c.initWithOpts(cfg, wmeta, sbom.ScanOptionsFromConfigForHosts(cfg))
}

func (c *Collector) initWithOpts(cfg config.Component, wmeta option.Option[workloadmeta.Component], opts sbom.ScanOptions) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg, wmeta)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	c.opts = opts
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	path := request.ID() // for host request, ID == path
	log.Infof("host scan request [%v]", path)

	report, err := c.DirectScan(ctx, path)
	return sbom.ScanResult{
		Error:            err,
		Report:           report,
		GenerationMethod: "filesystem",
	}
}

// DirectScan performs a scan on a specific path
func (c *Collector) DirectScan(ctx context.Context, path string) (sbom.Report, error) {
	return c.trivyCollector.ScanFilesystem(ctx, path, c.opts, true)
}

// DirectScanForTrivyReport performs a scan on a specific path
func (c *Collector) DirectScanForTrivyReport(ctx context.Context, path string) (*types.Report, error) {
	report, err := c.trivyCollector.ScanFSTrivyReport(ctx, path, c.opts, true)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// NewCollectorForCWS creates a new host collector, specifically for CWS
func NewCollectorForCWS(cfg config.Component, opts sbom.ScanOptions) (*Collector, error) {
	c := &Collector{
		resChan: make(chan sbom.ScanResult, channelSize),
	}

	if err := c.initWithOpts(cfg, option.None[workloadmeta.Component](), opts); err != nil {
		return nil, err
	}

	return c, nil
}
