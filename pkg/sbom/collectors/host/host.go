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

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
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
