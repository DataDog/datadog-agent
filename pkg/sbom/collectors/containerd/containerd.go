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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/containerd/containerd"
)

const (
	collectorName = "containerd"
)

// ScanRequest defines a scan request
type ScanRequest struct {
	ImageMeta        *workloadmeta.ContainerImageMetadata
	Image            containerd.Image
	ContainerdClient cutil.ContainerdItf
	FromFilesystem   bool
}

// Collector returns the collector name
func (r *ScanRequest) Collector() string {
	return collectorName
}

// Type returns the scan request type
func (r *ScanRequest) Type() string {
	if r.FromFilesystem {
		return sbom.ScanFilesystemType
	}
	return sbom.ScanDaemonType
}

// ID returns the scan request ID
func (r *ScanRequest) ID() string {
	return r.ImageMeta.ID
}

// Collector defines a containerd collector
type Collector struct {
	trivyCollector *trivy.Collector
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return c.trivyCollector.CleanCache()
}

// Init initializes the collector
func (c *Collector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

// Scan performs the scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) sbom.ScanResult {
	containerdScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)}
	}

	if containerdScanRequest.ImageMeta != nil {
		log.Infof("containerd scan request [%v]: scanning image %v", containerdScanRequest.ID(), containerdScanRequest.ImageMeta.Name)
	}

	var report sbom.Report
	var err error
	if containerdScanRequest.FromFilesystem {
		report, err = c.trivyCollector.ScanContainerdImageFromFilesystem(
			ctx,
			containerdScanRequest.ImageMeta,
			containerdScanRequest.Image,
			containerdScanRequest.ContainerdClient,
			opts,
		)
	} else {
		report, err = c.trivyCollector.ScanContainerdImage(
			ctx,
			containerdScanRequest.ImageMeta,
			containerdScanRequest.Image,
			containerdScanRequest.ContainerdClient,
			opts,
		)
	}
	scanResult := sbom.ScanResult{
		Error:   err,
		Report:  report,
		ImgMeta: containerdScanRequest.ImageMeta,
	}

	return scanResult
}

func init() {
	collectors.RegisterCollector(collectorName, &Collector{})
}
