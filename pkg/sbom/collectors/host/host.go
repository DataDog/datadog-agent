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

type ScanRequest struct {
	Path string
}

func (r *ScanRequest) Collector() string {
	return collectorName
}

func (r *ScanRequest) Type() string {
	return sbom.ScanFilesystemType
}

func (r *ScanRequest) ID() string {
	return r.Path
}

type HostCollector struct {
	trivyCollector *trivy.Collector
}

// CleanCache cleans the cache
func (c *HostCollector) CleanCache() error {
	return c.trivyCollector.CleanCache()
}

func (c *HostCollector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

func (c *HostCollector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) sbom.ScanResult {
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
	collectors.RegisterCollector(collectorName, &HostCollector{})
}
