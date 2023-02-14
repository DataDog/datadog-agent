// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/containerd/containerd"
)

// Collector interface
type Collector interface {
	ScanContainerdImage(ctx context.Context, imageMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error)
	ScanContainerdImageFromFilesystem(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error)
	Close() error
}
