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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

const (
	collectorName = "host"
)

// ScanRequest defines a scan request
type ScanRequest struct {
	Path string
}

// Collector returns the collector name
func (r *ScanRequest) Collector() string {
	return collectorName
}

// Type returns the scan request type
func (r *ScanRequest) Type() string {
	return sbom.ScanFilesystemType
}

// ID returns the scan request ID
func (r *ScanRequest) ID() string {
	return r.Path
}

// Collector defines a host collector
type Collector struct {
	trivyCollector *trivy.Collector
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) sbom.ScanResult {
	hostScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)}
	}
	log.Infof("host scan request [%v]", hostScanRequest.ID())

	report, err := c.trivyCollector.ScanFilesystem(ctx, hostScanRequest.Path, opts)
	return sbom.ScanResult{
		Error:  err,
		Report: report,
	}
}

func init() {
	collectors.RegisterCollector(collectorName, &Collector{})
}
