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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorName = "docker"
)

// ScanRequest exported type should have comment or be unexported
type ScanRequest struct {
	ImageMeta    *workloadmeta.ContainerImageMetadata
	DockerClient client.ImageAPIClient
}

// Collector exported method should have comment or be unexported
func (r *ScanRequest) Collector() string {
	return collectorName
}

// Type exported method should have comment or be unexported
func (r *ScanRequest) Type() string {
	return "daemon"
}

// ID exported method should have comment or be unexported
func (r *ScanRequest) ID() string {
	return r.ImageMeta.ID
}

// Collector exported type should have comment or be unexported
type Collector struct {
	trivyCollector *trivy.Collector
}

// CleanCache exported method should have comment or be unexported
func (c *Collector) CleanCache() error {
	return c.trivyCollector.GetCacheCleaner().Clean()
}

// Init exported method should have comment or be unexported
func (c *Collector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

// Scan exported method should have comment or be unexported
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
