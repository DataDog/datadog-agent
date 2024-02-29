// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"fmt"
	"os"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
)

const (
	datadogPackageLayerMediaType types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageMaxSize                        = 3 << 30 // 3GiB
)

type installer struct {
	repository *repository.Repository
}

func newInstaller(repository *repository.Repository) *installer {
	return &installer{
		repository: repository,
	}
}

func (i *installer) installStable(version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = extractPackageLayers(image, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	return i.repository.Create(version, tmpDir)
}

func (i *installer) installExperiment(version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = extractPackageLayers(image, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	return i.repository.SetExperiment(version, tmpDir)
}

func (i *installer) promoteExperiment() error {
	return i.repository.PromoteExperiment()
}

func (i *installer) uninstallExperiment() error {
	return i.repository.DeleteExperiment()
}

func extractPackageLayers(image oci.Image, dir string) error {
	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("could not get image layers: %w", err)
	}
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("could not get layer media type: %w", err)
		}
		if mediaType == datadogPackageLayerMediaType {
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = extractTarArchive(uncompressedLayer, dir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		}
	}
	return nil
}
