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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const (
	// PackagesPath is the path to the packages directory.
	PackagesPath = "/opt/datadog-packages"
	// TmpDirPath is the path to the temporary directory used for package installation.
	TmpDirPath = "/opt/datadog-packages"
	// LocksPack is the path to the locks directory.
	LocksPack = "/var/run/datadog-packages"

	datadogPackageMaxSize = 3 << 30 // 3GiB
	defaultConfigsDir     = "/etc"

	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageDatadogInstaller = "datadog-installer"

	mininumDiskSpace = datadogPackageMaxSize + 100<<20 // 3GiB + 100MiB
)

var (
	fsDisk = filesystem.NewDisk()
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

	downloader   *oci.Downloader
	repositories *repository.Repositories
	configsDir   string
	tmpDirPath   string
	packagesPath string
}

// Option are the options for the package manager.
type Option func(*options)

type options struct {
	registryAuth string
	registry     string
}

func newOptions() *options {
	return &options{
		registryAuth: oci.RegistryAuthDefault,
		registry:     "",
	}
}

// WithRegistryAuth sets the registry authentication method.
func WithRegistryAuth(registryAuth string) Option {
	return func(o *options) {
		o.registryAuth = registryAuth
	}
}

// WithRegistry sets the registry URL.
func WithRegistry(registry string) Option {
	return func(o *options) {
		o.registry = registry
	}
}

// NewInstaller returns a new Package Manager.
func NewInstaller(opts ...Option) Installer {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}
	return &installerImpl{
		downloader:   oci.NewDownloader(http.DefaultClient, o.registry, o.registryAuth),
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
	err := checkAvailableDiskSpace(mininumDiskSpace, i.packagesPath)
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
	err = pkg.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = pkg.ExtractLayers(oci.DatadogPackageConfigLayerMediaType, configDir)
	if err != nil {
		return fmt.Errorf("could not extract package config layer: %w", err)
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
	err := checkAvailableDiskSpace(mininumDiskSpace, i.packagesPath)
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
	err = pkg.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = pkg.ExtractLayers(oci.DatadogPackageConfigLayerMediaType, configDir)
	if err != nil {
		return fmt.Errorf("could not extract package config layer: %w", err)
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
	case packageDatadogInstaller:
		return service.SetupInstaller(ctx, true)
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
		return service.RemoveAgent(ctx)
	case packageAPMInjector:
		return service.RemoveAPMInjector(ctx)
	default:
		return nil
	}
}

// checkAvailableDiskSpace checks if there is enough disk space at the given paths.
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func checkAvailableDiskSpace(requiredDiskSpace uint64, paths ...string) error {
	for _, path := range paths {
		_, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("could not stat path %s: %w", path, err)
		}
		s, err := fsDisk.GetUsage(path)
		if err != nil {
			return err
		}
		if s.Available < uint64(requiredDiskSpace) {
			return fmt.Errorf("not enough disk space at %s: %d bytes available, %d bytes required", path, s.Available, requiredDiskSpace)
		}
	}
	return nil
}
