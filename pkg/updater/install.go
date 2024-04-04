// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"fmt"
	"os"
	"path/filepath"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/updater/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogPackageLayerMediaType       types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
	datadogPackageMaxSize                              = 3 << 30 // 3GiB
	defaultConfigsDir                                  = "/etc"

	packageDatadogAgent = "datadog-agent"
	packageAPMInjector  = "datadog-apm-inject"
)

type installer struct {
	repositories *repository.Repositories
	configsDir   string
}

func newInstaller(repositories *repository.Repositories) *installer {
	return &installer{
		repositories: repositories,
		configsDir:   defaultConfigsDir,
	}
}

func (i *installer) installStable(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = i.repositories.Create(pkg, version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}

	switch pkg {
	case packageDatadogAgent:
		return service.SetupAgentUnits()
	case packageAPMInjector:
		return service.SetupAPMInjector()
	default:
		return nil
	}
}

func (i *installer) installExperiment(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
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
	switch pkg {
	case packageDatadogAgent:
		return service.StartAgentExperiment()
	case packageAPMInjector:
		return service.StartAPMInjectorExperiment()
	default:
		return nil
	}
}

func (i *installer) stopExperiment(pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.StopAgentExperiment()
	case packageAPMInjector:
		return service.StopAPMInjectorExperiment()
	default:
		return nil
	}
}

func extractPackageLayers(image oci.Image, configDir string, packageDir string) error {
	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("could not get image layers: %w", err)
	}
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("could not get layer media type: %w", err)
		}
		switch mediaType {
		case datadogPackageLayerMediaType:
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = extractTarArchive(uncompressedLayer, packageDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		case datadogPackageConfigLayerMediaType:
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = extractTarArchive(uncompressedLayer, configDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		default:
			log.Warnf("can't install unsupported layer media type: %s", mediaType)
		}
	}
	return nil
}
