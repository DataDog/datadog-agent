// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && docker

package trivy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	containersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"

	dimage "github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

// buildDockerLayerPaths pairs DiffID and Path from the same overlay2
// inspect response. Digest is left empty: the daemon exposes no
// reliable per-layer manifest digest, and Trivy does not need one.
func buildDockerLayerPaths(inspect dimage.InspectResponse) ([]ftypes.LayerPath, error) {
	var paths []string
	if dirs, ok := inspect.GraphDriver.Data["LowerDir"]; ok && dirs != "" {
		parts := strings.Split(dirs, ":")
		for i := len(parts) - 1; i >= 0; i-- {
			paths = append(paths, parts[i])
		}
	}
	if dirs, ok := inspect.GraphDriver.Data["UpperDir"]; ok && dirs != "" {
		paths = append(paths, strings.Split(dirs, ":")...)
	}

	if env.IsContainerized() {
		for i, p := range paths {
			paths[i] = containersimage.SanitizeHostPath(p)
		}
	}

	diffIDs := inspect.RootFS.Layers
	if len(paths) != len(diffIDs) {
		return nil, fmt.Errorf("%w: %d paths vs %d diff_ids", errLayerCountMismatch, len(paths), len(diffIDs))
	}

	out := make([]ftypes.LayerPath, len(diffIDs))
	for i := range diffIDs {
		out[i] = ftypes.LayerPath{DiffID: diffIDs[i], Path: paths[i]}
	}
	return out, nil
}

// DockerCollector defines the docker collector name
const DockerCollector = "docker"

// Custom code based on https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/docker.go `DockerImage`
func convertDockerImage(ctx context.Context, client client.ImageAPIClient, imgMeta *workloadmeta.ContainerImageMetadata) (*image, func(), error) {
	cleanup := func() {}

	// <image_name>:<tag> pattern like "alpine:3.15"
	// or
	// <image_name>@<digest> pattern like "alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300"
	imageID := imgMeta.Name
	inspectResult, err := client.ImageInspect(ctx, imageID)
	if err != nil {
		imageID = imgMeta.ID // <image_id> pattern like `5ac716b05a9c`
		inspectResult, err = client.ImageInspect(ctx, imageID)
		if err != nil {
			return nil, cleanup, fmt.Errorf("unable to inspect the image (%s): %w", imageID, err)
		}
	}

	historyResult, err := client.ImageHistory(ctx, imageID)
	if err != nil {
		return nil, cleanup, fmt.Errorf("unable to get history (%s): %w", imageID, err)
	}

	f, err := os.CreateTemp("", "fanal-docker-*")
	if err != nil {
		return nil, cleanup, errors.New("failed to create a temporary file")
	}

	cleanup = func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}

	img := &image{
		name:    imgMeta.Name,
		opener:  imageOpener(ctx, DockerCollector, imageID, f, client.ImageSave),
		inspect: inspectResult.InspectResponse,
		history: configHistory(historyResult.Items),
	}

	return img, cleanup, nil
}

type fakeDockerContainer struct {
	*image
	*fakeContainer
}

func (c *fakeDockerContainer) LayerByDiffID(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDiffID(hash)
}

func (c *fakeDockerContainer) LayerByDigest(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDigest(hash)
}

func (c *fakeDockerContainer) Layers() (layers []ftypes.LayerPath) {
	return c.fakeContainer.Layers()
}

// ScanDockerImage scans a docker image by exporting it and scanning the tarball
func (c *Collector) ScanDockerImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client client.ImageAPIClient, scanOptions sbom.ScanOptions) (*Report, string, error) {
	fanalImage, cleanup, err := convertDockerImage(ctx, client, imgMeta)
	if cleanup != nil {
		defer cleanup()
	}

	if err != nil {
		return nil, "", fmt.Errorf("unable to convert docker image, err: %w", err)
	}

	if scanOptions.OverlayFsScan && fanalImage.inspect.GraphDriver.Name == "overlay2" {
		layers, err := buildDockerLayerPaths(fanalImage.inspect)
		if err != nil {
			return nil, "overlayfs", fmt.Errorf("unable to pair layer paths for image %s: %w", imgMeta.ID, err)
		}
		fakeContainer := &fakeDockerContainer{
			image:         fanalImage,
			fakeContainer: newFakeContainer(layers, imgMeta),
		}

		paths := make([]string, len(layers))
		for i, l := range layers {
			paths[i] = l.Path
		}
		report, err := c.scanOverlayFS(ctx, paths, fakeContainer, imgMeta, scanOptions)
		if err != nil {
			return nil, "overlayfs", err
		}

		return report, "overlayfs", nil
	}

	report, err := c.scanImage(ctx, fanalImage, scanOptions)
	if err != nil {
		return nil, "tarball", err
	}

	return report, "tarball", nil
}
