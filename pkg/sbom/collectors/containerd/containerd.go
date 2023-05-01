// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy
// +build containerd,trivy

package containerd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/containerd/containerd"
)

const (
	collectorName = "containerd"
)

type ScanRequest struct {
	ImageMeta        *workloadmeta.ContainerImageMetadata
	Image            containerd.Image
	ContainerdClient cutil.ContainerdItf
	FromFilesystem   bool
}

func (r *ScanRequest) Collector() string {
	return collectorName
}

func (r *ScanRequest) Type() string {
	if r.FromFilesystem {
		return sbom.ScanFilesystemType
	} else {
		return sbom.ScanDaemonType
	}
}

func (r *ScanRequest) ID() string {
	return r.ImageMeta.ID
}

type ContainerdCollector struct {
	trivyCollector *trivy.Collector
}

func (c *ContainerdCollector) CleanCache() error {
	return c.trivyCollector.GetCacheCleaner().Clean()
}

func (c *ContainerdCollector) Init(cfg config.Config) error {
	trivyCollector, err := trivy.GetGlobalCollector(cfg)
	if err != nil {
		return err
	}
	c.trivyCollector = trivyCollector
	return nil
}

func (c *ContainerdCollector) Scan(ctx context.Context, request sbom.ScanRequest, opts sbom.ScanOptions) (sbom.Report, error) {
	containerdScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)
	}

	if containerdScanRequest.FromFilesystem {
		return c.trivyCollector.ScanContainerdImageFromFilesystem(
			ctx,
			containerdScanRequest.ImageMeta,
			containerdScanRequest.Image,
			containerdScanRequest.ContainerdClient,
			opts,
		)
	} else {
		return c.trivyCollector.ScanContainerdImage(
			ctx,
			containerdScanRequest.ImageMeta,
			containerdScanRequest.Image,
			containerdScanRequest.ContainerdClient,
			opts,
		)
	}
}

func init() {
	collectors.RegisterCollector(collectorName, &ContainerdCollector{})
}
