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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/cdn"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/db"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageAPMLibraries     = "datadog-apm-libraries"
	packageDatadogInstaller = "datadog-installer"
)

// Installer is a package manager that installs and uninstalls packages.
type Installer interface {
	IsInstalled(ctx context.Context, pkg string) (bool, error)

	AvailableDiskSpace() (uint64, error)
	State(pkg string) (repository.State, error)
	States() (map[string]repository.State, error)
	ConfigState(pkg string) (repository.State, error)
	ConfigStates() (map[string]repository.State, error)

	Install(ctx context.Context, url string, args []string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	InstallConfigExperiment(ctx context.Context, pkg string, version string) error
	RemoveConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error

	InstrumentAPMInjector(ctx context.Context, method string) error
	UninstrumentAPMInjector(ctx context.Context, method string) error

	Close() error
}

// installerImpl is the implementation of the package manager.
type installerImpl struct {
	m sync.Mutex

	env        *env.Env
	cdn        *cdn.CDN
	db         *db.PackagesDB
	downloader *oci.Downloader
	packages   *repository.Repositories
	configs    *repository.Repositories

	packagesDir    string
	userConfigsDir string
}

// NewInstaller returns a new Package Manager.
func NewInstaller(env *env.Env) (Installer, error) {
	err := ensureRepositoriesExist()
	if err != nil {
		return nil, fmt.Errorf("could not ensure packages and config directory exists: %w", err)
	}
	db, err := db.New(filepath.Join(paths.PackagesPath, "packages.db"), db.WithTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("could not create packages db: %w", err)
	}
	cdn, err := cdn.New(env, filepath.Join(paths.RunPath, "rc_cmd"))
	if err != nil {
		return nil, fmt.Errorf("could not create CDN client: %w", err)
	}
	return &installerImpl{
		env:        env,
		cdn:        cdn,
		db:         db,
		downloader: oci.NewDownloader(env, env.HTTPClient()),
		packages:   repository.NewRepositories(paths.PackagesPath, paths.LocksPath),
		configs:    repository.NewRepositories(paths.ConfigsPath, paths.LocksPath),

		userConfigsDir: paths.DefaultUserConfigsDir,
		packagesDir:    paths.PackagesPath,
	}, nil
}

// AvailableDiskSpace returns the available disk space.
func (i *installerImpl) AvailableDiskSpace() (uint64, error) {
	return i.packages.AvailableDiskSpace()
}

// State returns the state of a package.
func (i *installerImpl) State(pkg string) (repository.State, error) {
	return i.packages.GetState(pkg)
}

// States returns the states of all packages.
func (i *installerImpl) States() (map[string]repository.State, error) {
	return i.packages.GetStates()
}

// ConfigState returns the state of a package.
func (i *installerImpl) ConfigState(pkg string) (repository.State, error) {
	return i.configs.GetState(pkg)
}

// ConfigStates returns the states of all packages.
func (i *installerImpl) ConfigStates() (map[string]repository.State, error) {
	return i.configs.GetStates()
}

// IsInstalled checks if a package is installed.
func (i *installerImpl) IsInstalled(_ context.Context, pkg string) (bool, error) {
	// The install script passes the package name as either <package>-<version> or <package>=<version>
	// depending on the platform so we strip the version prefix by looking for the "real" package name
	hasMatch := false
	for _, p := range PackagesList {
		if strings.HasPrefix(pkg, p.Name) {
			if hasMatch {
				return false, fmt.Errorf("the package %v matches multiple known packages", pkg)
			}
			pkg = p.Name
			hasMatch = true
		}
	}
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
	pkg, err := i.downloader.Download(ctx, url) // Downloads pkg metadata only
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	span, ok := telemetry.SpanFromContext(ctx)
	if ok {
		span.SetResourceName(pkg.Name)
		span.SetTag("package_version", pkg.Version)
	}
	err = i.preparePackage(ctx, pkg.Name, args) // Preinst
	if err != nil {
		return fmt.Errorf("could not prepare package: %w", err)
	}
	dbPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	}
	if dbPkg.Name == pkg.Name && dbPkg.Version == pkg.Version {
		log.Infof("package %s version %s is already installed", pkg.Name, pkg.Version)
		return nil
	}
	err = checkAvailableDiskSpace(i.packages, pkg)
	if err != nil {
		return fmt.Errorf("not enough disk space: %w", err)
	}
	tmpDir, err := i.packages.MkdirTemp()
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = i.db.DeletePackage(pkg.Name)
	if err != nil {
		return fmt.Errorf("could not remove package installation in db: %w", err)
	}
	configDir := filepath.Join(i.userConfigsDir, pkg.Name)
	err = pkg.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = pkg.ExtractLayers(oci.DatadogPackageConfigLayerMediaType, configDir)
	if err != nil {
		return fmt.Errorf("could not extract package config layer: %w", err)
	}
	err = i.packages.Create(pkg.Name, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	err = i.configurePackage(ctx, pkg.Name) // Config
	if err != nil {
		return fmt.Errorf("could not configure package: %w", err)
	}
	if pkg.Name == packageDatadogInstaller {
		// We must handle the configuration of some packages that are not
		// don't have an OCI. To properly configure their configuration repositories,
		// we call configurePackage when setting up the installer; which is the only
		// package that is always installed.
		err = i.configurePackage(ctx, packageAPMLibraries)
		if err != nil {
			return fmt.Errorf("could not configure package: %w", err)
		}
	}
	err = i.setupPackage(ctx, pkg.Name, args) // Postinst
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
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	err = checkAvailableDiskSpace(i.packages, pkg)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrNotEnoughDiskSpace,
			fmt.Errorf("not enough disk space: %w", err),
		)
	}
	tmpDir, err := i.packages.MkdirTemp()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could create temporary directory: %w", err),
		)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.userConfigsDir, pkg.Name)
	err = pkg.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not extract package layer: %w", err),
		)
	}
	err = pkg.ExtractLayers(oci.DatadogPackageConfigLayerMediaType, configDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not extract package config layer: %w", err),
		)
	}
	repository := i.packages.Get(pkg.Name)
	err = repository.SetExperiment(pkg.Version, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not set experiment: %w", err),
		)
	}

	return i.startExperiment(ctx, pkg.Name)
}

// RemoveExperiment removes an experiment.
func (i *installerImpl) RemoveExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.packages.Get(pkg)
	if runtime.GOOS != "windows" && pkg == packageDatadogInstaller {
		// Special case for the Linux installer since `stopExperiment`
		// will kill the current process, delete the experiment first.
		err := repository.DeleteExperiment()
		if err != nil {
			return installerErrors.Wrap(
				installerErrors.ErrFilesystemIssue,
				fmt.Errorf("could not delete experiment: %w", err),
			)
		}
		err = i.stopExperiment(ctx, pkg)
		if err != nil {
			return fmt.Errorf("could not stop experiment: %w", err)
		}
	} else {
		err := i.stopExperiment(ctx, pkg)
		if err != nil {
			return fmt.Errorf("could not stop experiment: %w", err)
		}
		err = repository.DeleteExperiment()
		if err != nil {
			return installerErrors.Wrap(
				installerErrors.ErrFilesystemIssue,
				fmt.Errorf("could not delete experiment: %w", err),
			)
		}
	}
	return nil
}

// PromoteExperiment promotes an experiment to stable.
func (i *installerImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.packages.Get(pkg)
	err := repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return i.promoteExperiment(ctx, pkg)
}

// InstallConfigExperiment installs an experiment on top of an existing package.
func (i *installerImpl) InstallConfigExperiment(ctx context.Context, pkg string, version string) error {
	i.m.Lock()
	defer i.m.Unlock()

	config, err := i.cdn.Get(ctx, pkg)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not get cdn config: %w", err),
		)
	}
	if config.State().GetVersion() != version {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("version mismatch: expected %s, got %s", config.State().GetVersion(), version),
		)
	}

	tmpDir, err := i.packages.MkdirTemp()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not create temporary directory: %w", err),
		)
	}
	defer os.RemoveAll(tmpDir)

	err = config.Write(tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not write agent config: %w", err),
		)
	}

	configRepo := i.configs.Get(pkg)
	err = configRepo.SetExperiment(version, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not set experiment: %w", err),
		)
	}

	switch runtime.GOOS {
	case "windows":
		return nil // TODO: start config experiment for Windows
	default:
		return i.startExperiment(ctx, pkg)
	}
}

// RemoveConfigExperiment removes an experiment.
func (i *installerImpl) RemoveConfigExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	err := i.stopExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	repository := i.configs.Get(pkg)
	err = repository.DeleteExperiment()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not delete experiment: %w", err),
		)
	}
	return nil
}

// PromoteConfigExperiment promotes an experiment to stable.
func (i *installerImpl) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.configs.Get(pkg)
	err := repository.PromoteExperiment()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not promote experiment: %w", err),
		)
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
	// NOTE: On Windows, purge must be called from a copy of the installer that
	//       exists outside of the install directory. If purge is called from
	//       within the installer package, then one of two things may happen:
	//       - this process will be ctrl-c killed and purge will not complete
	//       - this process will ignore ctrl-c and msiexec will kill msiserver,
	//         failing the uninstall.
	//       We can't workaround this by moving removePackage to the end of purge,
	//       as the daemon may be running and holding locks on files that need to be removed.
	err = i.removePackage(ctx, packageDatadogInstaller)
	if err != nil {
		log.Warnf("could not remove installer: %v", err)
	}

	// Must close dependencies before removing the rest of the files,
	// as some may be open/locked by the dependencies
	i.close()

	err = os.RemoveAll(paths.ConfigsPath)
	if err != nil {
		log.Warnf("could not delete configs dir: %v", err)
	}

	// explicitly remove the packages database from disk
	// It's in the packagesDir which we'll completely remove below,
	// however RemoveAll stops on the first failure and we want to
	// avoid leaving a stale database behind. Any stale repository
	// files will simply be removed by the next Install, but the packages.db
	// is still used as a source of truth.
	err = os.Remove(filepath.Join(i.packagesDir, "packages.db"))
	if err != nil {
		log.Warnf("could not delete packages db: %v", err)
	}

	// remove all from disk
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_all")
	err = os.RemoveAll(i.packagesDir)
	defer span.Finish(err)
	if err != nil {
		log.Warnf("could not delete packages dir: %v", err)
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
	err = i.packages.Delete(ctx, pkg)
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
func (i *installerImpl) GarbageCollect(_ context.Context) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := i.packages.Cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup packages: %w", err)
	}
	err = i.configs.Cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup configs: %w", err)
	}
	return nil
}

// InstrumentAPMInjector instruments the APM injector.
func (i *installerImpl) InstrumentAPMInjector(ctx context.Context, method string) error {
	i.m.Lock()
	defer i.m.Unlock()

	injectorInstalled, err := i.IsInstalled(ctx, packageAPMInjector)
	if err != nil {
		return fmt.Errorf("could not check if APM injector is installed: %w", err)
	}
	if !injectorInstalled {
		return fmt.Errorf("APM injector is not installed")
	}

	err = packages.InstrumentAPMInjector(ctx, method)
	if err != nil {
		return fmt.Errorf("could not instrument APM: %w", err)
	}
	return nil
}

// UninstrumentAPMInjector instruments the APM injector.
func (i *installerImpl) UninstrumentAPMInjector(ctx context.Context, method string) error {
	i.m.Lock()
	defer i.m.Unlock()

	injectorInstalled, err := i.IsInstalled(ctx, packageAPMInjector)
	if err != nil {
		return fmt.Errorf("could not check if APM injector is installed: %w", err)
	}
	if !injectorInstalled {
		return fmt.Errorf("APM injector is not installed")
	}

	err = packages.UninstrumentAPMInjector(ctx, method)
	if err != nil {
		return fmt.Errorf("could not instrument APM: %w", err)
	}
	return nil
}

// Close cleans up the Installer's dependencies, lock must be held by the caller
func (i *installerImpl) close() error {
	var errs []error

	if i.db != nil {
		if dbErr := i.db.Close(); dbErr != nil {
			dbErr = fmt.Errorf("failed to close packages database: %w", dbErr)
			errs = append(errs, dbErr)
		}
		i.db = nil
	}
	if i.cdn != nil {
		if cdnErr := i.cdn.Close(); cdnErr != nil {
			cdnErr = fmt.Errorf("failed to close Remote Config cdn: %w", cdnErr)
			errs = append(errs, cdnErr)
		}
		i.cdn = nil
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// Close cleans up the Installer's dependencies
func (i *installerImpl) Close() error {
	i.m.Lock()
	defer i.m.Unlock()
	return i.close()
}

func (i *installerImpl) startExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return packages.StartAgentExperiment(ctx)
	case packageDatadogInstaller:
		return packages.StartInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) stopExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return packages.StopAgentExperiment(ctx)
	case packageDatadogInstaller:
		return packages.StopInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) promoteExperiment(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return packages.PromoteAgentExperiment(ctx)
	case packageDatadogInstaller:
		return packages.PromoteInstallerExperiment(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) preparePackage(ctx context.Context, pkg string, _ []string) error {
	switch pkg {
	case packageDatadogInstaller:
		return packages.PrepareInstaller(ctx)
	case packageDatadogAgent:
		return packages.PrepareAgent(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) setupPackage(ctx context.Context, pkg string, args []string) error {
	switch pkg {
	case packageDatadogInstaller:
		return packages.SetupInstaller(ctx)
	case packageDatadogAgent:
		return packages.SetupAgent(ctx, args)
	case packageAPMInjector:
		return packages.SetupAPMInjector(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) removePackage(ctx context.Context, pkg string) error {
	switch pkg {
	case packageDatadogAgent:
		return packages.RemoveAgent(ctx)
	case packageAPMInjector:
		return packages.RemoveAPMInjector(ctx)
	case packageDatadogInstaller:
		return packages.RemoveInstaller(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) configurePackage(ctx context.Context, pkg string) (err error) {
	if !i.env.RemotePolicies {
		return nil
	}

	span, _ := telemetry.StartSpanFromContext(ctx, "configure_package")
	defer func() { span.Finish(err) }()

	switch pkg {
	case packageDatadogAgent, packageAPMInjector, packageAPMLibraries:
		config, err := i.cdn.Get(ctx, pkg)
		if err != nil {
			return fmt.Errorf("could not get %s CDN config: %w", pkg, err)
		}
		tmpDir, err := i.configs.MkdirTemp()
		if err != nil {
			return fmt.Errorf("could not create temporary directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		err = config.Write(tmpDir)
		if err != nil {
			return fmt.Errorf("could not write %s config: %w", pkg, err)
		}
		err = i.configs.Create(pkg, config.State().GetVersion(), tmpDir)
		if err != nil {
			return fmt.Errorf("could not create %s repository: %w", pkg, err)
		}
		return nil
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
func checkAvailableDiskSpace(repositories *repository.Repositories, pkg *oci.DownloadedPackage) error {
	requiredDiskSpace := pkg.Size
	if requiredDiskSpace == 0 {
		requiredDiskSpace = packageUnknownSize
	}
	requiredDiskSpace += installerOverhead

	availableDiskSpace, err := repositories.AvailableDiskSpace()
	if err != nil {
		return fmt.Errorf("could not get available disk space: %w", err)
	}
	if availableDiskSpace < uint64(requiredDiskSpace) {
		return fmt.Errorf("not enough disk space at %s: %d bytes available, %d bytes required", repositories.RootPath(), availableDiskSpace, requiredDiskSpace)
	}
	return nil
}

func ensureRepositoriesExist() error {
	err := os.MkdirAll(paths.PackagesPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating packages directory: %w", err)
	}
	err = os.MkdirAll(paths.ConfigsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating configs directory: %w", err)
	}
	return nil
}
