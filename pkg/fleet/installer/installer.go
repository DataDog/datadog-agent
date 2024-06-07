// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer provides a package manager that installs and uninstalls packages.
package installer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageDatadogInstaller = "datadog-installer"
)

var (
	fsDisk = filesystem.NewDisk()
)

// Installer is a package manager that installs and uninstalls packages.
type Installer interface {
	IsInstalled(ctx context.Context, pkg string) (bool, error)
	State(pkg string) (repository.State, error)
	States() (map[string]repository.State, error)

	Install(ctx context.Context, url string, args []string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error
}

// installerImpl is the implementation of the package manager.
type installerImpl struct {
	m sync.Mutex

	db           *db.PackagesDB
	downloader   *oci.Downloader
	repositories *repository.Repositories
	configsDir   string
	packagesDir  string
	tmpDirPath   string
}

// NewInstaller returns a new Package Manager.
func NewInstaller(env *env.Env) (Installer, error) {
	err := ensurePackageDirExists()
	if err != nil {
		return nil, fmt.Errorf("could not ensure packages directory exists: %w", err)
	}
	db, err := db.New(filepath.Join(PackagesPath, "packages.db"), db.WithTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("could not create packages db: %w", err)
	}
	return &installerImpl{
		db:           db,
		downloader:   oci.NewDownloader(env, http.DefaultClient),
		repositories: repository.NewRepositories(PackagesPath, LocksPack),
		configsDir:   DefaultConfigsDir,
		tmpDirPath:   TmpDirPath,
		packagesDir:  PackagesPath,
	}, nil
}

// State returns the state of a package.
func (i *installerImpl) State(pkg string) (repository.State, error) {
	return i.repositories.GetPackageState(pkg)
}

// States returns the states of all packages.
func (i *installerImpl) States() (map[string]repository.State, error) {
	return i.repositories.GetState()
}

// IsInstalled checks if a package is installed.
func (i *installerImpl) IsInstalled(_ context.Context, pkg string) (bool, error) {
	hasPackage, err := i.db.HasPackage(pkg)
	if err != nil {
		return false, fmt.Errorf("could not list packages: %w", err)
	}
	return hasPackage, nil
}

// Install installs or updates a package.
func (i *installerImpl) Install(ctx context.Context, url string, args []string) error {
	i.m.Lock()
	defer i.m.Unlock()
	pkg, err := i.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	dbPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	}
	if dbPkg.Name == pkg.Name && dbPkg.Version == pkg.Version {
		log.Infof("package %s version %s is already installed", pkg.Name, pkg.Version)
		return nil
	}
	err = checkAvailableDiskSpace(pkg, i.packagesDir)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}
	tmpDir, err := os.MkdirTemp(i.tmpDirPath, fmt.Sprintf("tmp-install-stable-%s-*", pkg.Name)) // * is replaced by a random string
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
	err = i.setupPackage(ctx, pkg.Name, args)
	if err != nil {
		return fmt.Errorf("could not setup package: %w", err)
	}
	err = i.db.SetPackage(db.Package{
		Name:             pkg.Name,
		Version:          pkg.Version,
		InstallerVersion: version.AgentVersion,
	})
	if err != nil {
		return fmt.Errorf("could not store package installation in db: %w", err)
	}
	return nil
}

// InstallExperiment installs an experiment on top of an existing package.
func (i *installerImpl) InstallExperiment(ctx context.Context, url string) error {
	i.m.Lock()
	defer i.m.Unlock()
	pkg, err := i.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	err = checkAvailableDiskSpace(pkg, i.packagesDir)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}
	tmpDir, err := os.MkdirTemp(i.tmpDirPath, fmt.Sprintf("tmp-install-experiment-%s-*", pkg.Name)) // * is replaced by a random string
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

	err := i.stopExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}

	repository := i.repositories.Get(pkg)
	err = repository.DeleteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return nil
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
	return i.promoteExperiment(ctx, pkg)
}

// Purge removes all packages.
func (i *installerImpl) Purge(ctx context.Context) {
	i.m.Lock()
	defer i.m.Unlock()

	packages, err := i.db.ListPackages()
	if err != nil {
		// if we can't list packages we'll only remove the installer
		packages = nil
		log.Warnf("could not list packages: %v", err)
	}
	for _, pkg := range packages {
		if pkg.Name == packageDatadogInstaller {
			continue
		}
		err := i.removePackage(ctx, pkg.Name)
		if err != nil {
			log.Warnf("could not remove package %s: %v", pkg.Name, err)
		}
	}
	err = i.removePackage(ctx, packageDatadogInstaller)
	if err != nil {
		log.Warnf("could not remove installer: %v", err)
	}

	// remove all from disk
	span, _ := tracer.StartSpanFromContext(ctx, "remove_all")
	err = os.RemoveAll(PackagesPath)
	defer span.Finish(tracer.WithError(err))
	if err != nil {
		log.Warnf("could not remove path: %v", err)
	}
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
		return fmt.Errorf("could not delete repository: %w", err)
	}
	err = i.db.DeletePackage(pkg)
	if err != nil {
		return fmt.Errorf("could not remove package installation in db: %w", err)
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

func (i *installerImpl) promoteExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return service.PromoteAgentExperiment(ctx)
	case packageDatadogInstaller:
		return service.StopInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) setupPackage(ctx context.Context, pkg string, args []string) error {
	switch pkg {
	case packageDatadogInstaller:
		return service.SetupInstaller(ctx)
	case packageDatadogAgent:
		return service.SetupAgent(ctx, args)
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
	case packageDatadogInstaller:
		return service.RemoveInstaller(ctx)
	default:
		return nil
	}
}

const (
	packageUnknownSize = 2 << 30  // 2GiB
	installerOverhead  = 10 << 20 // 10MiB
)

// checkAvailableDiskSpace checks if there is enough disk space to install a package at the given path.
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func checkAvailableDiskSpace(pkg *oci.DownloadedPackage, path string) error {
	requiredDiskSpace := pkg.Size
	if requiredDiskSpace == 0 {
		requiredDiskSpace = packageUnknownSize
	}
	requiredDiskSpace += installerOverhead

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
	return nil
}

func ensurePackageDirExists() error {
	err := os.MkdirAll(PackagesPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating packages directory: %w", err)
	}
	return nil
}
