// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && trivy
// +build docker,trivy

package docker

import (
	"context"
	"fmt"
	"reflect"

	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorName = "docker-collector"
)

type ScanRequest struct {
	ImageMeta    *workloadmeta.ContainerImageMetadata
	DockerClient client.ImageAPIClient
}

func (r *ScanRequest) Collector() string {
	return collectorName
}

func (r *ScanRequest) Type() string {
	return "daemon"
}

type Collector struct {
	trivyCollector *trivy.Collector
}

func (c *Collector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

func (c *Collector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) (sbom.Report, error) {
	dockerScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)
	}

	return c.trivyCollector.ScanDockerImage(
		ctx,
		dockerScanRequest.ImageMeta,
		dockerScanRequest.DockerClient,
		opts,
	)
}

func init() {
	collectors.RegisterCollector(collectorName, &Collector{})
}
