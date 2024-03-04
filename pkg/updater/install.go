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
	"github.com/DataDog/datadog-agent/pkg/updater/service"
)

const (
	datadogPackageLayerMediaType types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageMaxSize                        = 3 << 30 // 3GiB
)

type installer struct {
	repositories *repository.Repositories
}

func newInstaller(repositories *repository.Repositories) *installer {
	return &installer{
		repositories: repositories,
	}
}

func (i *installer) installStable(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = extractPackageLayers(image, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	return i.repositories.Create(pkg, version, tmpDir)
}

func (i *installer) installExperiment(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = extractPackageLayers(image, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	repository := i.repositories.Get(pkg)
	err = repository.SetExperiment(version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return i.startExperiment(pkg)
}

func (i *installer) promoteExperiment(pkg string) error {
	repository := i.repositories.Get(pkg)
	err := repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return i.stopExperiment(pkg)
}

func (i *installer) uninstallExperiment(pkg string) error {
	repository := i.repositories.Get(pkg)
	err := repository.DeleteExperiment()
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return i.stopExperiment(pkg)
}

func (i *installer) startExperiment(pkg string) error {
	// TODO(arthur): currently we only support the datadog-agent package
	if pkg != "datadog-agent" {
		return nil
	}
	return service.StartAgentExperiment()
}

func (i *installer) stopExperiment(pkg string) error {
	// TODO(arthur): currently we only support the datadog-agent package
	if pkg != "datadog-agent" {
		return nil
	}
	return service.StopAgentExperiment()
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
