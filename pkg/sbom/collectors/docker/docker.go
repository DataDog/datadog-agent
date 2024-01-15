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

	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

const (
	collectorName = "docker"
)

// ScanRequest defines a scan request
type ScanRequest struct {
	ImageMeta    *workloadmeta.ContainerImageMetadata
	DockerClient client.ImageAPIClient
}

// GetImgMetadata returns the image metadata
func (r *ScanRequest) GetImgMetadata() *workloadmeta.ContainerImageMetadata {
	return r.ImageMeta
}

// Collector returns the collector name
func (r *ScanRequest) Collector() string {
	return collectorName
}

// Type returns the scan request type
func (r *ScanRequest) Type() string {
	return "daemon"
}

// ID returns the scan request ID
func (r *ScanRequest) ID() string {
	return r.ImageMeta.ID
}

// Collector defines a collector
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

// Scan performs a scan
func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) sbom.ScanResult {
	dockerScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)}
	}

	if dockerScanRequest.ImageMeta != nil {
		log.Infof("docker scan request [%v]: scanning image %v", dockerScanRequest.ID(), dockerScanRequest.ImageMeta.Name)
	}

	report, err := c.trivyCollector.ScanDockerImage(
		ctx,
		dockerScanRequest.ImageMeta,
		dockerScanRequest.DockerClient,
		opts,
	)

	return sbom.ScanResult{
		Error:   err,
		Report:  report,
		ImgMeta: dockerScanRequest.ImageMeta,
	}
}

func init() {
	collectors.RegisterCollector(collectorName, &Collector{})
}
