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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	packageDatadogAgent     = "datadog-agent"
	packageAPMInjector      = "datadog-apm-inject"
	packageDatadogInstaller = "datadog-installer"
	packageAPMLibraryDotnet = "datadog-apm-library-dotnet"
)

// Installer is a package manager that installs and uninstalls packages.
type Installer interface {
	IsInstalled(ctx context.Context, pkg string) (bool, error)

	AvailableDiskSpace() (uint64, error)
	State(ctx context.Context, pkg string) (repository.State, error)
	States(ctx context.Context) (map[string]repository.State, error)
	ConfigState(ctx context.Context, pkg string) (repository.State, error)
	ConfigStates(ctx context.Context) (map[string]repository.State, error)

	Install(ctx context.Context, url string, extensions []string, args []string) error
	ForceInstall(ctx context.Context, url string, extensions []string, args []string) error
	SetupInstaller(ctx context.Context, path string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error // TODO: handle extensions
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations) error
	RemoveConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	InstallExtensions(ctx context.Context, url string, extensions []string) error
	RemoveExtensions(ctx context.Context, pkg string, extensions []string) error

	GarbageCollect(ctx context.Context) error

	InstrumentAPMInjector(ctx context.Context, method string) error
	UninstrumentAPMInjector(ctx context.Context, method string) error

	Close() error
}

// installerImpl is the implementation of the package manager.
type installerImpl struct {
	m sync.Mutex

	env        *env.Env
	db         *db.PackagesDB
	downloader *oci.Downloader
	packages   *repository.Repositories
	config     *config.Directories
	hooks      packages.Hooks

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
	pkgs := repository.NewRepositories(paths.PackagesPath, packages.AsyncPreRemoveHooks)
	i := &installerImpl{
		env:        env,
		db:         db,
		downloader: oci.NewDownloader(env, env.HTTPClient()),
		packages:   pkgs,
		config: &config.Directories{
			StablePath:     paths.AgentConfigDir,
			ExperimentPath: paths.AgentConfigDir + "-exp",
		},
		hooks: packages.NewHooks(env, pkgs),

		userConfigsDir: paths.DefaultUserConfigsDir,
		packagesDir:    paths.PackagesPath,
	}
	return i, nil
}

// AvailableDiskSpace returns the available disk space.
func (i *installerImpl) AvailableDiskSpace() (uint64, error) {
	return i.packages.AvailableDiskSpace()
}

// State returns the state of a package.
func (i *installerImpl) State(_ context.Context, pkg string) (repository.State, error) {
	return i.packages.GetState(pkg)
}

// States returns the states of all packages.
func (i *installerImpl) States(_ context.Context) (map[string]repository.State, error) {
	return i.packages.GetStates()
}

// ConfigState returns the state of a package.
func (i *installerImpl) ConfigState(_ context.Context, _ string) (repository.State, error) {
	state, err := i.config.GetState()
	if err != nil {
		return repository.State{}, fmt.Errorf("could not get config state: %w", err)
	}
	return repository.State{
		Stable:     state.StableDeploymentID,
		Experiment: state.ExperimentDeploymentID,
	}, nil
}

// ConfigStates returns the states of all packages.
func (i *installerImpl) ConfigStates(_ context.Context) (map[string]repository.State, error) {
	state, err := i.config.GetState()
	if err != nil {
		return nil, fmt.Errorf("could not get config state: %w", err)
	}
	stableDeploymentID := state.StableDeploymentID
	if stableDeploymentID == "" {
		stableDeploymentID = "empty"
	}
	return map[string]repository.State{
		"datadog-agent": {
			Stable:     stableDeploymentID,
			Experiment: state.ExperimentDeploymentID,
		},
	}, nil
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

// ForceInstall installs or updates a package, even if it's already installed
func (i *installerImpl) ForceInstall(ctx context.Context, url string, extensions []string, args []string) error {
	return i.doInstall(ctx, url, extensions, args, func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool {
		if dbPkg.Name == pkg.Name && dbPkg.Version == pkg.Version {
			log.Warnf("package %s version %s is already installed, updating it anyway", pkg.Name, pkg.Version)
		}
		return true
	})
}

// Install installs or updates a package.
func (i *installerImpl) Install(ctx context.Context, url string, extensions []string, args []string) error {
	return i.doInstall(ctx, url, extensions, args, func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool {
		if dbPkg.Name == pkg.Name && dbPkg.Version == pkg.Version {
			log.Warnf("package %s version %s is already installed", pkg.Name, pkg.Version)
			return false
		}
		return true
	})
}

// SetupInstaller with given path sets up the installer/agent package.
func (i *installerImpl) SetupInstaller(ctx context.Context, path string) error {
	i.m.Lock()
	defer i.m.Unlock()

	// make sure data directory is set up correctly
	err := paths.EnsureInstallerDataDir()
	if err != nil {
		return fmt.Errorf("could not ensure installer data directory permissions: %w", err)
	}

	_, err = i.db.GetPackage(packageDatadogAgent)
	if err == nil {
		// need to remove the agent before installing the installer
		err = i.db.DeletePackage(packageDatadogAgent)
		if err != nil {
			return fmt.Errorf("could not remove agent: %w", err)
		}

	} else if !errors.Is(err, db.ErrPackageNotFound) {
		// there was a real error
		return fmt.Errorf("could not get package: %w", err)
	}

	// remove the agent from the repository no matter database state
	pkgState, err := i.packages.Get(packageDatadogAgent).GetState()
	if err != nil {
		return fmt.Errorf("could not get agent state: %w", err)
	}

	// need to make sure there is an agent package
	// in the repository before we can call Delete
	if pkgState.HasStable() {
		err = i.packages.Delete(ctx, packageDatadogAgent)
		if err != nil {
			return fmt.Errorf("could not delete agent repository: %w", err)
		}
	}

	// if windows we need to copy the MSI to temp directory
	if runtime.GOOS == "windows" {
		// copy the MSI to the temp directory
		tmpDir, err := i.packages.MkdirTemp()
		if err != nil {
			return fmt.Errorf("could not create temporary directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		msiName := fmt.Sprintf("datadog-agent-%s-x86_64.msi", version.AgentPackageVersion)
		err = paths.CopyFile(path, filepath.Join(tmpDir, msiName))
		if err != nil {
			return fmt.Errorf("could not copy installer: %w", err)
		}
		path = tmpDir
	}

	// create the installer package
	err = i.packages.Create(ctx, packageDatadogAgent, version.AgentPackageVersion, path)
	if err != nil {
		return fmt.Errorf("could not create installer repository: %w", err)
	}

	// add to the db
	err = i.db.SetPackage(db.Package{
		Name:             packageDatadogAgent,
		Version:          version.AgentPackageVersion,
		InstallerVersion: version.AgentVersion,
	})
	if err != nil {
		return fmt.Errorf("could not store package installation in db: %w", err)
	}

	return nil

}

func (i *installerImpl) doInstall(ctx context.Context, url string, extensions []string, args []string, shouldInstallPredicate func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool) error {
	i.m.Lock()
	defer i.m.Unlock()
	pkg, err := i.downloader.Download(ctx, url) // Downloads pkg metadata only
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	span, ok := telemetry.SpanFromContext(ctx)
	if ok {
		span.SetResourceName(pkg.Name)
		span.SetTag("package_version", pkg.Version)
	}
	dbPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	}
	if shouldInstallPredicate(dbPkg, pkg) {
		upgrade := !errors.Is(err, db.ErrPackageNotFound) && dbPkg.Version != pkg.Version
		if upgrade {
			err = i.hooks.PreRemove(ctx, pkg.Name, packages.PackageTypeOCI, true)
			if err != nil {
				return fmt.Errorf("could not prepare package: %w", err)
			}
		}
		err = i.hooks.PreInstall(ctx, pkg.Name, packages.PackageTypeOCI, upgrade)
		if err != nil {
			return fmt.Errorf("could not prepare package: %w", err)
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
			return fmt.Errorf("could not create temporary directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		err = i.db.DeletePackage(pkg.Name)
		if err != nil {
			return fmt.Errorf("could not remove package installation in db: %w", err)
		}
		configDir := filepath.Join(i.userConfigsDir, "datadog-agent")
		err = pkg.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
		if err != nil {
			return fmt.Errorf("could not extract package layers: %w", err)
		}
		err = pkg.ExtractLayers(oci.DatadogPackageConfigLayerMediaType, configDir)
		if err != nil {
			return fmt.Errorf("could not extract package config layer: %w", err)
		}
		err = i.packages.Create(ctx, pkg.Name, pkg.Version, tmpDir)
		if err != nil {
			return fmt.Errorf("could not create repository: %w", err)
		}
		err = i.hooks.PostInstall(ctx, pkg.Name, packages.PackageTypeOCI, upgrade, args)
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
	}

	if len(extensions) > 0 {
		// note: cannot call i.InstallExtensions here (it locks), so use the split-out logic.
		err = i.installExtensions(ctx, pkg, extensions)
		if err != nil {
			return fmt.Errorf("could not install extensions, package still installed: %w", err)
		}
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
	configDir := filepath.Join(i.userConfigsDir, "datadog-agent")
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

	err = i.hooks.PreStartExperiment(ctx, pkg.Name)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	repository := i.packages.Get(pkg.Name)
	err = repository.SetExperiment(ctx, pkg.Version, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not set experiment: %w", err),
		)
	}
	err = i.config.RemoveExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not remove config experiment: %w", err)
	}
	// HACK: close so package can be updated as watchdog runs
	if pkg.Name == packageDatadogAgent && runtime.GOOS == "windows" {
		i.db.Close()
	}
	err = i.hooks.PostStartExperiment(ctx, pkg.Name)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	return nil
}

// RemoveExperiment removes an experiment.
func (i *installerImpl) RemoveExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.packages.Get(pkg)
	state, err := repository.GetState()
	if err != nil {
		return fmt.Errorf("could not get repository state: %w", err)
	}
	if !state.HasExperiment() {
		// Return early
		return nil
	}

	if runtime.GOOS != "windows" && (pkg == packageDatadogInstaller || pkg == packageDatadogAgent) {
		// Special case for the Linux installer since `preStopExperiment`
		// will kill the current process, delete the experiment first.
		err := repository.DeleteExperiment(ctx)
		if err != nil {
			return installerErrors.Wrap(
				installerErrors.ErrFilesystemIssue,
				fmt.Errorf("could not delete experiment: %w", err),
			)
		}
		err = i.hooks.PreStopExperiment(ctx, pkg)
		if err != nil {
			return fmt.Errorf("could not stop experiment: %w", err)
		}
	} else {
		err = i.hooks.PreStopExperiment(ctx, pkg)
		if err != nil {
			return fmt.Errorf("could not stop experiment: %w", err)
		}
		err = repository.DeleteExperiment(ctx)
		if err != nil {
			return installerErrors.Wrap(
				installerErrors.ErrFilesystemIssue,
				fmt.Errorf("could not delete experiment: %w", err),
			)
		}
	}
	err = i.hooks.PostStopExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	return nil
}

// PromoteExperiment promotes an experiment to stable.
func (i *installerImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	repository := i.packages.Get(pkg)
	state, err := repository.GetState()
	if err != nil {
		return fmt.Errorf("could not get repository state: %w", err)
	}
	if !state.HasExperiment() {
		// Fail early
		return fmt.Errorf("no experiment to promote")
	}

	err = i.hooks.PrePromoteExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}

	err = repository.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}

	err = i.hooks.PostPromoteExperiment(ctx, pkg)
	if err != nil {
		return err
	}

	// Update db
	state, err = repository.GetState()
	if err != nil {
		return fmt.Errorf("could not get repository state: %w", err)
	}
	return i.db.SetPackage(db.Package{
		Name:             pkg,
		Version:          state.Stable,
		InstallerVersion: version.AgentVersion,
	})
}

// InstallConfigExperiment installs an experiment on top of an existing package.
func (i *installerImpl) InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations) error {
	i.m.Lock()
	defer i.m.Unlock()

	err := i.packages.Get(pkg).DeleteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	err = i.config.WriteExperiment(ctx, operations)
	if err != nil {
		return fmt.Errorf("could not write experiment: %w", err)
	}

	// HACK: close so package can be updated as watchdog runs
	if pkg == packageDatadogAgent && runtime.GOOS == "windows" {
		i.db.Close()
	}

	return i.hooks.PostStartConfigExperiment(ctx, pkg)
}

// RemoveConfigExperiment removes an experiment.
func (i *installerImpl) RemoveConfigExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	err := i.hooks.PreStopConfigExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	err = i.config.RemoveExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not remove experiment: %w", err)
	}
	return nil
}

// PromoteConfigExperiment promotes an experiment to stable.
func (i *installerImpl) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()

	err := i.config.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return i.hooks.PostPromoteConfigExperiment(ctx, pkg)
}

// Purge removes all packages.
func (i *installerImpl) Purge(ctx context.Context) {
	i.m.Lock()
	defer i.m.Unlock()

	dbPackages, err := i.db.ListPackages()
	if err != nil {
		// if we can't list packages we'll only remove the installer
		dbPackages = nil
		log.Warnf("could not list packages: %v", err)
	}
	for _, pkg := range dbPackages {
		if pkg.Name == packageDatadogInstaller {
			continue
		}
		if pkg.Name == packageDatadogAgent {
			continue
		}
		err := i.hooks.PreRemove(ctx, pkg.Name, packages.PackageTypeOCI, false)
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
	err = i.hooks.PreRemove(ctx, packageDatadogAgent, packages.PackageTypeOCI, false)
	if err != nil {
		log.Warnf("could not remove agent: %v", err)
	}
	// TODO: wont need this when Linux packages are merged
	if runtime.GOOS != "windows" {
		// on windows the installer package has been merged with the agent package
		err = i.hooks.PreRemove(ctx, packageDatadogInstaller, packages.PackageTypeOCI, false)
		if err != nil {
			log.Warnf("could not remove installer: %v", err)
		}
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

	err = purgeTmpDirectory(paths.RootTmpDir)
	if err != nil {
		log.Warnf("could not delete tmp directory: %v", err)
	}
}

// Remove uninstalls a package.
func (i *installerImpl) Remove(ctx context.Context, pkg string) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := i.hooks.PreRemove(ctx, pkg, packages.PackageTypeOCI, false)
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
func (i *installerImpl) GarbageCollect(ctx context.Context) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := i.packages.Cleanup(ctx)
	if err != nil {
		return fmt.Errorf("could not cleanup packages: %w", err)
	}
	err = cleanupTmpDirectory(paths.RootTmpDir)
	if err != nil {
		return fmt.Errorf("could not cleanup tmp directory: %w", err)
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

// InstallExtensions installs multiple extensions.
func (i *installerImpl) InstallExtensions(ctx context.Context, url string, extensions []string) error {
	i.m.Lock()
	defer i.m.Unlock()

	if len(extensions) == 0 {
		return nil
	}

	pkg, err := i.downloader.Download(ctx, url) // Downloads pkg metadata only
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	span, ok := telemetry.SpanFromContext(ctx)
	if ok {
		span.SetResourceName(fmt.Sprintf("%s_extensions", pkg.Name))
		span.SetTag("package_name", pkg.Name)
		span.SetTag("package_version", pkg.Version)
		span.SetTag("extensions", strings.Join(extensions, ","))
		span.SetTag("url", url)
	}
	return i.installExtensions(ctx, pkg, extensions)
}

func (i *installerImpl) installExtensions(ctx context.Context, pkg *oci.DownloadedPackage, extensions []string) error {
	dbPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	} else if err != nil && errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("package %s not found, cannot install extension", pkg.Name)
	} else if err == nil && dbPkg.Version != pkg.Version {
		return fmt.Errorf("cannot install extension version %s, version mismatch with package %s version %s", pkg.Name, pkg.Version, dbPkg.Version)
	}

	// Initialize extensions map if needed
	if dbPkg.Extensions == nil {
		dbPkg.Extensions = make(map[string]struct{})
	}

	// Track which extensions were successfully installed for rollback
	var installedExtensions []string
	var installErrors []error

	// Process each extension
	for _, extension := range extensions {
		// Check if extension is already installed
		if _, exists := dbPkg.Extensions[extension]; exists {
			log.Infof("Extension %s already installed, skipping", extension)
			continue
		}

		err := i.installExtension(ctx, pkg, extension)
		if err != nil {
			installErrors = append(installErrors, err)
			continue
		}

		// Mark as installed
		dbPkg.Extensions[extension] = struct{}{}
		installedExtensions = append(installedExtensions, extension)
	}

	// Update package in DB if any extensions were installed
	if len(installedExtensions) > 0 {
		err = i.db.SetPackage(dbPkg)
		if err != nil {
			// Clean up on failure
			for _, extension := range installedExtensions {
				extractDir := filepath.Join(i.packagesDir, pkg.Name, pkg.Version, "ext", extension)
				os.RemoveAll(extractDir)
			}
			return fmt.Errorf("could not update package in db: %w", err)
		}
	}

	// If all extensions failed, return error
	if len(installErrors) == len(extensions) {
		return errors.Join(installErrors...)
	}

	// If some extensions failed, log warnings but don't fail
	if len(installErrors) > 0 {
		for _, err := range installErrors {
			log.Warnf("Extension installation error: %v", err)
		}
	}

	return nil
}

// installExtension installs a single extension for a package.
func (i *installerImpl) installExtension(ctx context.Context, pkg *oci.DownloadedPackage, extension string) error {
	err := i.hooks.PreInstallExtension(ctx, pkg.Name, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	// Extract to a temporary directory first
	tmpDir, err := i.packages.MkdirTemp()
	if err != nil {
		return fmt.Errorf("could not create temp directory for %s: %w", extension, err)
	}
	defer os.RemoveAll(tmpDir)

	err = pkg.ExtractLayers(oci.DatadogPackageExtensionLayerMediaType, tmpDir, oci.LayerAnnotation{Key: "com.datadoghq.package.extension.name", Value: extension})
	if err != nil {
		return fmt.Errorf("could not extract layers for %s: %w", extension, err)
	}

	extractDir := filepath.Join(i.packagesDir, pkg.Name, pkg.Version, "ext", extension)
	if err := os.MkdirAll(filepath.Dir(extractDir), 0755); err != nil {
		return fmt.Errorf("could not create directory for %s: %w", extension, err)
	}

	err = os.Rename(tmpDir, extractDir)
	if err != nil {
		return fmt.Errorf("could not move %s to final location: %w", extension, err)
	}

	err = i.hooks.PostInstallExtension(ctx, pkg.Name, extension)
	if err != nil {
		return fmt.Errorf("could not install extension: %w", err)
	}

	return nil
}

// RemoveExtensions removes multiple extensions.
func (i *installerImpl) RemoveExtensions(ctx context.Context, pkg string, extensions []string) error {
	i.m.Lock()
	defer i.m.Unlock()

	if len(extensions) == 0 {
		return nil
	}

	dbPkg, err := i.db.GetPackage(pkg)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	} else if err != nil && errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("package %s not found, cannot remove extension", pkg)
	}

	// Track which extensions were successfully removed
	var removedExtensions []string
	var removeErrors []error

	// Process each extension
	for _, extension := range extensions {
		// Check if extension is installed
		if _, exists := dbPkg.Extensions[extension]; !exists {
			log.Infof("Extension %s not installed, skipping", extension)
			continue
		}

		err := i.removeExtension(ctx, pkg, dbPkg.Version, extension)
		if err != nil {
			removeErrors = append(removeErrors, err)
			continue
		}

		// Mark as removed
		delete(dbPkg.Extensions, extension)
		removedExtensions = append(removedExtensions, extension)
	}

	// Update package in DB if any extensions were removed
	if len(removedExtensions) > 0 {
		err = i.db.SetPackage(dbPkg)
		if err != nil {
			return fmt.Errorf("could not update package in db: %w", err)
		}
	}

	// If all extensions failed, return error
	if len(removeErrors) == len(extensions) {
		return errors.Join(removeErrors...)
	}

	// If some extensions failed, log warnings but don't fail
	if len(removeErrors) > 0 {
		for _, err := range removeErrors {
			log.Warnf("Extension removal error: %v", err)
		}
	}

	return nil
}

// removeExtension removes a single extension for a package.
func (i *installerImpl) removeExtension(ctx context.Context, pkg, version, extension string) error {
	err := i.hooks.PreRemoveExtension(ctx, pkg, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	extensionDir := filepath.Join(i.packagesDir, pkg, version, "ext", extension)
	err = os.RemoveAll(extensionDir)
	if err != nil {
		return fmt.Errorf("could not remove directory for %s: %w", extension, err)
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

// ensureRepositoriesExist creates the temp, packages and configs directories if they don't exist
func ensureRepositoriesExist() error {
	// TODO: should we call paths.EnsureInstallerDataDir() here?
	//       It should probably be anywhere that the below directories must be
	//       created, but it feels wrong to have the constructor perform work
	//       like this. For example, "read only" subcommands like `get-states`
	//       will end up iterating the filesystem tree to apply permissions,
	//       and every subprocess during experiments will repeat the work,
	//       even though it should only be needed at install/setup time.
	err := os.MkdirAll(paths.PackagesPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating packages directory: %w", err)
	}
	err = os.MkdirAll(paths.ConfigsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating configs directory: %w", err)
	}
	err = os.MkdirAll(paths.RootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating tmp directory: %w", err)
	}
	err = os.MkdirAll(paths.RunPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating tmp directory: %w", err)
	}

	return nil
}

// cleanupTmpDirectory removes files and directories in RootTmpDir that are older than 24 hours
func cleanupTmpDirectory(rootTmpDir string) error {
	// Check if RootTmpDir exists
	if _, err := os.Stat(rootTmpDir); os.IsNotExist(err) {
		// Directory doesn't exist, nothing to clean up
		return nil
	}

	// Calculate the cutoff time (24 hours ago)
	cutoffTime := time.Now().Add(-24 * time.Hour)

	// Read the directory contents
	entries, err := os.ReadDir(rootTmpDir)
	if err != nil {
		return fmt.Errorf("could not read tmp directory: %w", err)
	}

	var cleanupErrors []string
	for _, entry := range entries {
		entryPath := filepath.Join(rootTmpDir, entry.Name())

		// Get file info to check modification time
		info, err := entry.Info()
		if err != nil {
			log.Warnf("Could not get info for %s: %v", entryPath, err)
			continue
		}

		// Check if the file/directory is older than 24 hours
		if info.ModTime().Before(cutoffTime) {
			log.Debugf("Removing old tmp file/directory: %s (modified: %v)", entryPath, info.ModTime())

			err := os.RemoveAll(entryPath)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("failed to remove %s: %v", entryPath, err))
				log.Warnf("Could not remove old tmp file/directory %s: %v", entryPath, err)
			} else {
				log.Debugf("Successfully removed old tmp file/directory: %s", entryPath)
			}
		}
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("tmp directory cleanup completed with errors: %s", strings.Join(cleanupErrors, "; "))
	}

	return nil
}

// purgeTmpDirectory removes the tmp directory
var purgeTmpDirectory = func(rootTmpDir string) error {
	err := os.RemoveAll(rootTmpDir)
	if err != nil {
		return err
	}
	return nil
}
