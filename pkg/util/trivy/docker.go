// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && docker

package trivy

import (
	"context"
	"fmt"
	"os"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	containersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"

	"github.com/docker/docker/client"
)

// DockerCollector defines the docker collector name
const DockerCollector = "docker"

// Custom code based on https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/docker.go `DockerImage`
func convertDockerImage(ctx context.Context, client client.ImageAPIClient, imgMeta *workloadmeta.ContainerImageMetadata) (*image, func(), error) {
	cleanup := func() {}

	// <image_name>:<tag> pattern like "alpine:3.15"
	// or
	// <image_name>@<digest> pattern like "alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300"
	imageID := imgMeta.Name
	inspect, err := client.ImageInspect(ctx, imageID)
	if err != nil {
		imageID = imgMeta.ID // <image_id> pattern like `5ac716b05a9c`
		inspect, err = client.ImageInspect(ctx, imageID)
		if err != nil {
			return nil, cleanup, fmt.Errorf("unable to inspect the image (%s): %w", imageID, err)
		}
	}

	history, err := client.ImageHistory(ctx, imageID)
	if err != nil {
		return nil, cleanup, fmt.Errorf("unable to get history (%s): %w", imageID, err)
	}

	f, err := os.CreateTemp("", "fanal-docker-*")
	if err != nil {
		return nil, cleanup, fmt.Errorf("failed to create a temporary file")
	}

	cleanup = func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}

	img := &image{
		opener:  imageOpener(ctx, DockerCollector, imageID, f, client.ImageSave),
		inspect: inspect,
		history: configHistory(history),
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
func (c *Collector) ScanDockerImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client client.ImageAPIClient, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertDockerImage(ctx, client, imgMeta)
	if cleanup != nil {
		defer cleanup()
	}

	if err != nil {
		return nil, fmt.Errorf("unable to convert docker image, err: %w", err)
	}

	if scanOptions.OverlayFsScan && fanalImage.inspect.GraphDriver.Name == "overlay2" {
		var layers []string
		if layerDirs, ok := fanalImage.inspect.GraphDriver.Data["LowerDir"]; ok {
			layers = append(layers, strings.Split(layerDirs, ":")...)
		}

		if layerDirs, ok := fanalImage.inspect.GraphDriver.Data["UpperDir"]; ok {
			layers = append(layers, strings.Split(layerDirs, ":")...)
		}

		if env.IsContainerized() {
			for i, layer := range layers {
				layers[i] = containersimage.SanitizeHostPath(layer)
			}
		}

		fakeContainer := &fakeDockerContainer{
			image: fanalImage,
			fakeContainer: &fakeContainer{
				layerIDs:   fanalImage.inspect.RootFS.Layers,
				layerPaths: layers,
				imgMeta:    imgMeta,
			},
		}

		return c.scanOverlayFS(ctx, layers, fakeContainer, imgMeta, scanOptions)
	}

	return c.scanImage(ctx, fanalImage, imgMeta, scanOptions)
}
