// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

// Package trivy holds the scan components
package trivy

import (
	"context"
	"fmt"
	"os"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/errdefs"
)

const (
	cleanupTimeout = 30 * time.Second
)

// ContainerdAccessor is a function that should return a containerd client
type ContainerdAccessor func() (cutil.ContainerdItf, error)

// ScanContainerdImageFromSnapshotter scans containerd image directly from the snapshotter
func (c *Collector) ScanContainerdImageFromSnapshotter(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))
	clClient := client.RawClient()
	imageID := imgMeta.ID

	mounts, err := client.Mounts(ctx, expiration, imgMeta.Namespace, img)
	if err != nil {
		return nil, fmt.Errorf("unable to get mounts for image %s, err: %w", imgMeta.ID, err)
	}

	layers := extractLayersFromOverlayFSMounts(mounts)
	if len(layers) == 0 {
		return nil, fmt.Errorf("unable to extract layers from overlayfs mounts %+v for image %s", mounts, imgMeta.ID)
	}

	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)
	// Adding a lease to cleanup dandling snaphots at expiration
	ctx, done, err := clClient.WithLease(ctx,
		leases.WithID(imageID),
		leases.WithExpiration(expiration),
		leases.WithLabels(map[string]string{
			"containerd.io/gc.ref.snapshot." + containerd.DefaultSnapshotter: imageID,
		}),
	)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return nil, fmt.Errorf("unable to get a lease, err: %w", err)
	}

	report, err := c.scanOverlayFS(ctx, layers, imgMeta, scanOptions)

	if err := done(ctx); err != nil {
		log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
	}

	return report, err
}

// ScanContainerdImage scans containerd image by exporting it and scanning the tarball
func (c *Collector) ScanContainerdImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertContainerdImage(ctx, client.RawClient(), imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("unable to convert containerd image, err: %w", err)
	}

	return c.scanImage(ctx, fanalImage, imgMeta, scanOptions)
}

// ScanContainerdImageFromFilesystem scans containerd image from file-system
func (c *Collector) ScanContainerdImageFromFilesystem(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	imagePath, err := os.MkdirTemp("", "containerd-image-*")
	if err != nil {
		return nil, fmt.Errorf("unable to create temp dir, err: %w", err)
	}
	defer func() {
		err := os.RemoveAll(imagePath)
		if err != nil {
			log.Errorf("Unable to remove temp dir: %s, err: %v", imagePath, err)
		}
	}()

	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))

	cleanUp, err := client.MountImage(ctx, expiration, imgMeta.Namespace, img, imagePath)
	if err != nil {
		return nil, fmt.Errorf("unable to mount containerd image, err: %w", err)
	}

	defer func() {
		cleanUpContext, cleanUpContextCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		err := cleanUp(cleanUpContext)
		cleanUpContextCancel()
		if err != nil {
			log.Errorf("Unable to clean up mounted image, err: %v", err)
		}
	}()

	return c.scanFilesystem(ctx, os.DirFS("/"), imagePath, imgMeta, scanOptions)
}
