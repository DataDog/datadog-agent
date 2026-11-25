// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy && containerd && docker && crio

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	containerd "github.com/containerd/containerd/v2/client"
)

// newCollector returns the collector of the scans, which builds the usage index
// of each SBOM with usageIndex.
func newCollector(usageIndex bool) *trivy.Collector {
	collector := trivy.NewCollectorForCLI()
	if usageIndex {
		collector.EnableUsageIndex()
	}
	return collector
}

func runScanFS(path string, analyzers []string, fast bool, usageIndex bool) error {
	collector := newCollector(usageIndex)

	ctx := context.Background()
	report, err := collector.ScanFilesystem(ctx, path, sbom.ScanOptions{
		Analyzers: analyzers,
		Fast:      fast,
	}, false)
	if err != nil {
		return err
	}

	return outputReport(report)
}

func runScanDocker(imageMeta *workloadmeta.ContainerImageMetadata, analyzers []string, fast bool, usageIndex bool) error {
	collector := newCollector(usageIndex)

	cl, err := docker.GetDockerUtil()
	if err != nil {
		return fmt.Errorf("error creating docker client: %w", err)
	}
	dockerClient := cl.RawClient()

	ctx := context.Background()
	report, _, err := collector.ScanDockerImage(
		ctx,
		imageMeta,
		dockerClient,
		sbom.ScanOptions{
			Analyzers: analyzers,
			Fast:      fast,
		},
	)
	if err != nil {
		return err
	}

	return outputReport(report)
}

func runScanContainerd(imageMeta *workloadmeta.ContainerImageMetadata, analyzers []string, fast bool, strategy string, usageIndex bool) error {
	collector := newCollector(usageIndex)

	containerdClient, err := containerdutil.NewContainerdUtil()
	if err != nil {
		return fmt.Errorf("error creating containerd client: %w", err)
	}

	image, err := containerdClient.Image(imageMeta.Namespace, imageMeta.Name)
	if err != nil {
		return fmt.Errorf("error getting image %s/%s: %w", imageMeta.Namespace, imageMeta.Name, err)
	}

	var report sbom.Report
	var scanner func(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client containerdutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error)
	switch strategy {
	case "overlayfs":
		scanner = collector.ScanContainerdImageFromSnapshotter
	case "image":
		scanner = collector.ScanContainerdImage
	default:
		return fmt.Errorf("unknown strategy: %s", strategy)
	}

	ctx := context.Background()
	report, err = scanner(ctx, imageMeta, image, containerdClient, sbom.ScanOptions{
		Analyzers: analyzers,
		Fast:      fast,
	})
	if err != nil {
		return err
	}

	return outputReport(report)
}

func runScanCrio(imageMeta *workloadmeta.ContainerImageMetadata, analyzers []string, fast bool, usageIndex bool) error {
	collector := newCollector(usageIndex)

	crioClient, err := crio.NewCRIOClient()
	if err != nil {
		return fmt.Errorf("error creating CRI-O client: %w", err)
	}

	ctx := context.Background()
	report, err := collector.ScanCRIOImageFromOverlayFS(
		ctx,
		imageMeta,
		crioClient,
		sbom.ScanOptions{
			Analyzers: analyzers,
			Fast:      fast,
		},
	)
	if err != nil {
		return err
	}

	return outputReport(report)
}

func outputReport(report sbom.Report) error {
	bom := report.ToCycloneDX()
	bomJSON, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("sbom: %+v\n", string(bomJSON))
	if index := sbom.UsageIndexOf(report); index != nil {
		fmt.Fprintf(os.Stderr, "usage index: %d components, %d anchors, %d bytes\n", index.Len(), index.Anchors(), index.Size())
	}
	return nil
}
