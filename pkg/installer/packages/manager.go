// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packages provides a package manager that installs and uninstalls packages.
package packages

import (
	"context"
	"fmt"
	"net/http"
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

// Manager is a package manager that installs and uninstalls packages.
type Manager interface {
	State(pkg string) (repository.State, error)
	States() (map[string]repository.State, error)

	Install(ctx context.Context, url string) error
	Remove(ctx context.Context, pkg string) error

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error
}

// managerImpl is the implementation of the package manager.
type managerImpl struct {
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

// NewManager returns a new Package Manager.
func NewManager(opts ...Options) Manager {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}
	return &managerImpl{
		downloader:   newDownloader(http.DefaultClient, o.registryAuth, o.registry),
		repositories: repository.NewRepositories(PackagesPath, LocksPack),
		configsDir:   defaultConfigsDir,
		tmpDirPath:   TmpDirPath,
		packagesPath: PackagesPath,
	}
}

// State returns the state of a package.
func (m *managerImpl) State(pkg string) (repository.State, error) {
	return m.repositories.GetPackageState(pkg)
}

// States returns the states of all packages.
func (m *managerImpl) States() (map[string]repository.State, error) {
	return m.repositories.GetState()
}

// Install installs or updates a package.
func (m *managerImpl) Install(ctx context.Context, url string) error {
	m.m.Lock()
	defer m.m.Unlock()
	err := utils.CheckAvailableDiskSpace(mininumDiskSpace, m.packagesPath)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}

	pkg, err := m.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	tmpDir, err := os.MkdirTemp(m.tmpDirPath, fmt.Sprintf("install-stable-%s-*", pkg.Name)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(m.configsDir, pkg.Name)
	err = extractPackageLayers(pkg.Image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = m.repositories.Create(ctx, pkg.Name, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	return m.setupPackage(ctx, pkg.Name)
}

// InstallExperiment installs an experiment on top of an existing package.
func (m *managerImpl) InstallExperiment(ctx context.Context, url string) error {
	m.m.Lock()
	defer m.m.Unlock()
	err := utils.CheckAvailableDiskSpace(mininumDiskSpace, m.packagesPath)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}

	pkg, err := m.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	tmpDir, err := os.MkdirTemp(m.tmpDirPath, fmt.Sprintf("install-experiment-%s-*", pkg.Name)) // * is replaced by a random string
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(m.configsDir, pkg.Name)
	err = extractPackageLayers(pkg.Image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	repository := m.repositories.Get(pkg.Name)
	err = repository.SetExperiment(ctx, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return m.startExperiment(ctx, pkg.Name)
}

// RemoveExperiment removes an experiment.
func (m *managerImpl) RemoveExperiment(ctx context.Context, pkg string) error {
	m.m.Lock()
	defer m.m.Unlock()

	repository := m.repositories.Get(pkg)
	err := repository.DeleteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return m.stopExperiment(ctx, pkg)
}

// PromoteExperiment promotes an experiment to stable.
func (m *managerImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	m.m.Lock()
	defer m.m.Unlock()

	repository := m.repositories.Get(pkg)
	err := repository.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return m.stopExperiment(ctx, pkg)
}

// Remove uninstalls a package.
func (m *managerImpl) Remove(ctx context.Context, pkg string) error {
	m.m.Lock()
	defer m.m.Unlock()

	err := m.removePackage(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}

	err = m.repositories.Delete(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}

// GarbageCollect removes unused packages.
func (m *managerImpl) GarbageCollect(ctx context.Context) error {
	m.m.Lock()
	defer m.m.Unlock()

	return m.repositories.Cleanup(ctx)
}

func (m *managerImpl) startExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.StartAgentExperiment(ctx)
	case packageDatadogInstaller:
		return service.StartInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (m *managerImpl) stopExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.StopAgentExperiment(ctx)
	case packageAPMInjector:
		return service.StopInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (m *managerImpl) setupPackage(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.SetupAgent(ctx)
	case packageAPMInjector:
		return service.SetupAPMInjector(ctx)
	default:
		return nil
	}
}

func (m *managerImpl) removePackage(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.RemoveAgent(ctx)
	case packageAPMInjector:
		return service.RemoveAPMInjector(ctx)
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
