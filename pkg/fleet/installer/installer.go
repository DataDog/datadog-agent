// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer provides a package manager that installs and uninstalls packages.
package installer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// PackagesPath is the path to the packages directory.
	PackagesPath = "/opt/datadog-packages"
	// TmpDirPath is the path to the temporary directory used for package installation.
	TmpDirPath = "/opt/datadog-packages"
	// LocksPack is the path to the locks directory.
	LocksPack = "/var/run/datadog-packages"

	datadogPackageLayerMediaType       types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
	datadogPackageMaxSize                              = 3 << 30 // 3GiB
	defaultConfigsDir                                  = "/etc"

	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageDatadogInstaller = "datadog-installer"

	mininumDiskSpace = datadogPackageMaxSize + 100<<20 // 3GiB + 100MiB
)

// Installer is a package manager that installs and uninstalls packages.
type Installer interface {
	State(pkg string) (repository.State, error)
	States() (map[string]repository.State, error)

	Install(ctx context.Context, url string) error
	Remove(ctx context.Context, pkg string) error

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error
}

// installerImpl is the implementation of the package manager.
type installerImpl struct {
	m sync.Mutex

	downloader   *downloader
	repositories *repository.Repositories
	configsDir   string
	tmpDirPath   string
	packagesPath string
}

// Options are the options for the package manager.
type Options func(*options)

type options struct {
	registryAuth RegistryAuth
	registry     string
}

func newOptions() *options {
	return &options{
		registryAuth: RegistryAuthDefault,
		registry:     "",
	}
}

// WithRegistryAuth sets the registry authentication method.
func WithRegistryAuth(registryAuth RegistryAuth) Options {
	return func(o *options) {
		o.registryAuth = registryAuth
	}
}

// WithRegistry sets the registry URL.
func WithRegistry(registry string) Options {
	return func(o *options) {
		o.registry = registry
	}
}

// NewInstaller returns a new Package Manager.
func NewInstaller(opts ...Options) Installer {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}
	return &installerImpl{
		downloader:   newDownloader(http.DefaultClient, o.registryAuth, o.registry),
		repositories: repository.NewRepositories(PackagesPath, LocksPack),
		configsDir:   defaultConfigsDir,
		tmpDirPath:   TmpDirPath,
		packagesPath: PackagesPath,
	}
}

// State returns the state of a package.
func (i *installerImpl) State(pkg string) (repository.State, error) {
	return i.repositories.GetPackageState(pkg)
}

// States returns the states of all packages.
func (i *installerImpl) States() (map[string]repository.State, error) {
	return i.repositories.GetState()
}

// Install installs or updates a package.
func (i *installerImpl) Install(ctx context.Context, url string) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := utils.CheckAvailableDiskSpace(mininumDiskSpace, i.packagesPath)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}

	pkg, err := i.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	tmpDir, err := os.MkdirTemp(i.tmpDirPath, fmt.Sprintf("install-stable-%s-*", pkg.Name)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg.Name)
	err = extractPackageLayers(pkg.Image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = i.repositories.Create(ctx, pkg.Name, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	return i.setupPackage(ctx, pkg.Name)
}

// InstallExperiment installs an experiment on top of an existing package.
func (i *installerImpl) InstallExperiment(ctx context.Context, url string) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := utils.CheckAvailableDiskSpace(mininumDiskSpace, i.packagesPath)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}

	pkg, err := i.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	tmpDir, err := os.MkdirTemp(i.tmpDirPath, fmt.Sprintf("install-experiment-%s-*", pkg.Name)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg.Name)
	err = extractPackageLayers(pkg.Image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	repository := i.repositories.Get(pkg.Name)
	err = repository.SetExperiment(ctx, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return i.startExperiment(ctx, pkg.Name)
}

// RemoveExperiment removes an experiment.
func (i *installerImpl) RemoveExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.repositories.Get(pkg)
	err := repository.DeleteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return i.stopExperiment(ctx, pkg)
}

// PromoteExperiment promotes an experiment to stable.
func (i *installerImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.repositories.Get(pkg)
	err := repository.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return i.stopExperiment(ctx, pkg)
}

// Remove uninstalls a package.
func (i *installerImpl) Remove(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	err := i.removePackage(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}

	err = i.repositories.Delete(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}

// GarbageCollect removes unused packages.
func (i *installerImpl) GarbageCollect(ctx context.Context) error {
	i.m.Lock()
	defer i.m.Unlock()

	return i.repositories.Cleanup(ctx)
}

func (i *installerImpl) startExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.StartAgentExperiment(ctx)
	case packageDatadogInstaller:
		return service.StartInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) stopExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.StopAgentExperiment(ctx)
	case packageAPMInjector:
		return service.StopInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) setupPackage(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.SetupAgent(ctx)
	case packageAPMInjector:
		return service.SetupAPMInjector(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) removePackage(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		service.RemoveAgent(ctx)
		return nil
	case packageAPMInjector:
		service.RemoveAPMInjector(ctx)
		return nil
	case packageDatadogInstaller:
		service.RemoveInstaller(ctx)
		return nil
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
