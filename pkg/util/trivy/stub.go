// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !trivy
// +build !trivy

package trivy

import (
	"context"
	"fmt"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/containerd/containerd"
)

// This implementation does nothing. It's for agents that do not need Trivy.
// Trivy increases the size noticeably, so we avoid requiring it if not needed.

// Collector interface
type Collector interface {
	ScanContainerdImage(ctx context.Context, imageMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error)
}

type collector struct {
}

type CollectorConfig struct {
	ContainerdAccessor func() (*containerd.Client, error)
}

func (c *collector) ScanContainerdImage(_ context.Context, _ *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error) {
	return nil, fmt.Errorf("not implemented")
}

func NewCollector(_ CollectorConfig) (Collector, error) {
	return &collector{}, nil
}

func DefaultCollectorConfig() (CollectorConfig, error) {
	return CollectorConfig{}, nil
}
