// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/installer/packages/repository"
	"github.com/DataDog/datadog-agent/pkg/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/installer/packages/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogPackageLayerMediaType       types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
	datadogPackageMaxSize                              = 3 << 30 // 3GiB
	defaultConfigsDir                                  = "/etc"

	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageDatadogInstaller = "datadog-installer"
)

type packageManager struct {
	repositories *repository.Repositories
	configsDir   string
	installLock  sync.Mutex
	tmpDirPath   string
}

func newPackageManager(repositories *repository.Repositories) *packageManager {
	return &packageManager{
		repositories: repositories,
		configsDir:   defaultConfigsDir,
		tmpDirPath:   defaultRepositoriesPath,
	}
}

func (m *packageManager) installStable(ctx context.Context, pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp(m.tmpDirPath, fmt.Sprintf("install-stable-%s-*", pkg)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(m.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = m.repositories.Create(ctx, pkg, version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	return m.setupUnits(ctx, pkg)
}

func (m *packageManager) setupUnits(ctx context.Context, pkg string) error {
	m.installLock.Lock()
	defer m.installLock.Unlock()
	switch pkg {
	case packageDatadogAgent:
		return service.SetupAgentUnits(ctx)
	case packageAPMInjector:
		return service.SetupAPMInjector(ctx)
	default:
		return nil
	}
}

func (m *packageManager) installExperiment(ctx context.Context, pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp(m.tmpDirPath, fmt.Sprintf("install-experiment-%s-*", pkg)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(m.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	repository := m.repositories.Get(pkg)
	err = repository.SetExperiment(ctx, version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return m.startExperiment(ctx, pkg)
}

func (m *packageManager) promoteExperiment(ctx context.Context, pkg string) error {
	repository := m.repositories.Get(pkg)
	err := repository.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return m.stopExperiment(ctx, pkg)
}

func (m *packageManager) uninstallExperiment(ctx context.Context, pkg string) error {
	repository := m.repositories.Get(pkg)
	err := repository.DeleteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return m.stopExperiment(ctx, pkg)
}

func (m *packageManager) startExperiment(ctx context.Context, pkg string) error {
	m.installLock.Lock()
	defer m.installLock.Unlock()
	switch pkg {
	case packageDatadogAgent:
		return service.StartAgentExperiment(ctx)
	case packageDatadogInstaller:
		return service.StartInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (m *packageManager) stopExperiment(ctx context.Context, pkg string) error {
	m.installLock.Lock()
	defer m.installLock.Unlock()
	switch pkg {
	case packageDatadogAgent:
		return service.StopAgentExperiment(ctx)
	case packageAPMInjector:
		return service.StopInstallerExperiment(ctx)
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
			err = utils.ExtractTarArchive(uncompressedLayer, packageDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		case datadogPackageConfigLayerMediaType:
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = utils.ExtractTarArchive(uncompressedLayer, configDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		default:
			log.Warnf("can't install unsupported layer media type: %s", mediaType)
		}
	}
	return nil
}
