// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/docker/docker/client"
	"golang.org/x/xerrors"
)

// Custom code based on https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/docker.go `DockerImage`
func convertDockerImage(ctx context.Context, client client.ImageAPIClient, imgMeta *workloadmeta.ContainerImageMetadata) (types.Image, func(), error) {
	cleanup := func() {}

	// <image_name>:<tag> pattern like "alpine:3.15"
	// or
	// <image_name>@<digest> pattern like "alpine@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300"
	imageID := imgMeta.Name
	inspect, _, err := client.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		imageID = imgMeta.ID // <image_id> pattern like `5ac716b05a9c`
		inspect, _, err = client.ImageInspectWithRaw(ctx, imageID)
		if err != nil {
			return nil, cleanup, xerrors.Errorf("unable to inspect the image (%s): %w", imageID, err)
		}
	}

	history, err := client.ImageHistory(ctx, imageID)
	if err != nil {
		return nil, cleanup, xerrors.Errorf("unable to get history (%s): %w", imageID, err)
	}

	f, err := os.CreateTemp("", "fanal-docker-*")
	if err != nil {
		return nil, cleanup, xerrors.Errorf("failed to create a temporary file")
	}

	cleanup = func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}

	return &image{
		opener:  imageOpener(ctx, imageID, f, client.ImageSave),
		inspect: inspect,
		history: configHistory(history),
	}, cleanup, nil
}
