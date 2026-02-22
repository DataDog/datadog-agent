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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
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
	ConfigState(ctx context.Context, pkg string) (repository.State, error)
	ConfigAndPackageStates(ctx context.Context) (*repository.PackageStates, error)

	Install(ctx context.Context, url string, args []string) error
	ForceInstall(ctx context.Context, url string, args []string) error
	SetupInstaller(ctx context.Context, path string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations, decryptedSecrets map[string]string) error
	RemoveConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	InstallExtensions(ctx context.Context, url string, extensionList []string) error
	RemoveExtensions(ctx context.Context, pkg string, extensionList []string) error
	SaveExtensions(ctx context.Context, pkg string, path string) error
	RestoreExtensions(ctx context.Context, url string, path string) error

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
	db, err := db.New(filepath.Join(paths.PackagesPath, "packages.db"), db.WithTimeout(5*time.Minute))
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
			ExperimentPath: paths.AgentConfigDirExp,
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

// configStates returns the config states of all supported packages (only datadog-agent is supported today).
func (i *installerImpl) configStates(_ context.Context) (map[string]repository.State, error) {
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

// ConfigAndPackageStates returns the states of all packages' configurations and packages.
func (i *installerImpl) ConfigAndPackageStates(ctx context.Context) (*repository.PackageStates, error) {
	configStates, err := i.configStates(ctx)
	if err != nil {
		return nil, err
	}
	packageStates, err := i.packages.GetStates()
	if err != nil {
		return nil, err
	}
	return &repository.PackageStates{
		ConfigStates: configStates,
		States:       packageStates,
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
func (i *installerImpl) ForceInstall(ctx context.Context, url string, args []string) error {
	return i.doInstall(ctx, url, args, func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool {
		if dbPkg.Name == pkg.Name && dbPkg.Version == pkg.Version {
			log.Warnf("package %s version %s is already installed, updating it anyway", pkg.Name, pkg.Version)
		}
		return true
	})
}

// Install installs or updates a package.
func (i *installerImpl) Install(ctx context.Context, url string, args []string) error {
	return i.doInstall(ctx, url, args, func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool {
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
	err := paths.SetupInstallerDataDir()
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

func (i *installerImpl) doInstall(ctx context.Context, url string, args []string, shouldInstallPredicate func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool) error {
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
	if !shouldInstallPredicate(dbPkg, pkg) {
		return nil
	}
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
	err = extensions.SetPackage(ctx, pkg.Name, pkg.Version, false)
	if err != nil {
		return fmt.Errorf("could not store package extensions in db: %w", err)
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
		return errors.New("no experiment to promote")
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
func (i *installerImpl) InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations, decryptedSecrets map[string]string) error {
	i.m.Lock()
	defer i.m.Unlock()

	// Replace secrets in operations
	if err := config.ReplaceSecrets(&operations, decryptedSecrets); err != nil {
		return fmt.Errorf("could not replace secrets: %w", err)
	}

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
		// Delete the package entry from the database.
		// This ensures the entry is removed even if os.Remove(packages.db) fails later
		// due to file locking race conditions on Windows.
		err = i.db.DeletePackage(pkg.Name)
		if err != nil {
			log.Warnf("could not delete package %s from db: %v", pkg.Name, err)
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
	//
	// Note: If DD_NO_AGENT_UNINSTALL is set, then the agent will not be uninstalled.
	//       This is used to prevent the agent from being uninstalled when purge is
	//       called from within the MSI.
	if uninstallAgent, ok := os.LookupEnv("DD_NO_AGENT_UNINSTALL"); !ok || strings.ToLower(uninstallAgent) != "true" {
		err = i.hooks.PreRemove(ctx, packageDatadogAgent, packages.PackageTypeOCI, false)
		if err != nil {
			log.Warnf("could not remove agent: %v", err)
		}
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
	// TODO: check if error must trigger a specific flow
	_ = i.close()

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
	err = extensions.DeletePackage(ctx, pkg, false)
	if err != nil {
		return fmt.Errorf("could not remove package from extensions db: %w", err)
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

	var err error
	if runtime.GOOS == "windows" && method == env.APMInstrumentationEnabledIIS {
		var isDotnetInstalled bool
		isDotnetInstalled, err = i.IsInstalled(ctx, packageAPMLibraryDotnet)
		if err != nil {
			return fmt.Errorf("could not check if APM dotnet library is installed: %w", err)
		}
		if !isDotnetInstalled {
			return errors.New("APM dotnet library is not installed")
		}
	} else {
		var isInjectorInstalled bool
		isInjectorInstalled, err = i.IsInstalled(ctx, packageAPMInjector)
		if err != nil {
			return fmt.Errorf("could not check if APM injector is installed: %w", err)
		}
		if !isInjectorInstalled {
			return errors.New("APM injector is not installed")
		}
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

	var err error
	if runtime.GOOS == "windows" && method == env.APMInstrumentationEnabledIIS {
		var isDotnetInstalled bool
		isDotnetInstalled, err = i.IsInstalled(ctx, packageAPMLibraryDotnet)
		if err != nil {
			return fmt.Errorf("could not check if APM dotnet library is installed: %w", err)
		}
		if !isDotnetInstalled {
			return errors.New("APM dotnet library is not installed")
		}
	} else {
		var isInjectorInstalled bool
		isInjectorInstalled, err = i.IsInstalled(ctx, packageAPMInjector)
		if err != nil {
			return fmt.Errorf("could not check if APM injector is installed: %w", err)
		}
		if !isInjectorInstalled {
			return errors.New("APM injector is not installed")
		}
	}

	err = packages.UninstrumentAPMInjector(ctx, method)
	if err != nil {
		return fmt.Errorf("could not uninstrument APM: %w", err)
	}
	return nil
}

// InstallExtensions installs multiple extensions.
func (i *installerImpl) InstallExtensions(ctx context.Context, url string, extensionList []string) error {
	i.m.Lock()
	defer i.m.Unlock()

	if len(extensionList) == 0 {
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
		span.SetResourceName("install_extensions")
		span.SetTag("package_name", pkg.Name)
		span.SetTag("package_version", pkg.Version)
		span.SetTag("extensions", strings.Join(extensionList, ","))
		span.SetTag("url", url)
	}

	existingPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package %s from database: %w", pkg.Name, err)
	}
	if existingPkg.Version != pkg.Version && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("package %s is installed at version %s, requested version is %s", pkg.Name, existingPkg.Version, pkg.Version)
	}

	err = extensions.Install(ctx, i.downloader, url, extensionList, false, i.hooks)
	if err != nil {
		return fmt.Errorf("could not install extensions: %w", err)
	}

	// Special case for Linux & datadog-agent: restart the Agent after installing Agent extensions.
	if pkg.Name == packageDatadogAgent {
		return packages.RestartDatadogAgent(ctx)
	}
	return nil
}

// RemoveExtensions removes multiple extensions.
func (i *installerImpl) RemoveExtensions(ctx context.Context, pkg string, extensionList []string) error {
	i.m.Lock()
	defer i.m.Unlock()

	if len(extensionList) == 0 {
		return nil
	}

	span, ok := telemetry.SpanFromContext(ctx)
	if ok {
		span.SetResourceName("remove_extensions")
		span.SetTag("package_name", pkg)
		span.SetTag("extensions", strings.Join(extensionList, ","))
	}

	err := extensions.Remove(ctx, pkg, extensionList, false, i.hooks)
	if err != nil {
		return fmt.Errorf("could not remove extensions: %w", err)
	}

	// Special case for Linux & datadog-agent: restart the Agent after removing Agent extensions.
	if pkg == packageDatadogAgent {
		return packages.RestartDatadogAgent(ctx)
	}
	return nil
}

// SaveExtensions saves the extensions to a specific location on disk.
func (i *installerImpl) SaveExtensions(ctx context.Context, pkg string, path string) error {
	i.m.Lock()
	defer i.m.Unlock()
	return extensions.Save(ctx, pkg, path)
}

// RestoreExtensions restores the extensions from a specific location on disk.
func (i *installerImpl) RestoreExtensions(ctx context.Context, url string, path string) error {
	i.m.Lock()
	defer i.m.Unlock()
	pkg, err := i.downloader.Download(ctx, url) // Downloads pkg metadata only
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	err = extensions.Restore(ctx, i.downloader, pkg.Name, url, path, false, i.hooks)
	if err != nil {
		return fmt.Errorf("could not restore extensions: %w", err)
	}

	// Special case for datadog-agent: restart the Agent after restoring Agent extensions manually.
	if pkg.Name == packageDatadogAgent {
		return packages.RestartDatadogAgent(ctx)
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
	// Enforce permissions on the installer data directory
	//
	// TODO: Avoid setup-like work in the installer constructor.
	// These directories should be created properly at install/setup time, however
	// the installer constructor is called before install/setup runs, and will fail if
	// these directories do not exist, so we must do some minimal creation/validation here.
	// We must avoid doing too much work here (e.g. iterating the filesystem tree)
	// because the constructor is called for every subprocess, even "read only"
	// subcommands like `get-states`.
	err := paths.EnsureInstallerDataDir()
	if err != nil {
		return fmt.Errorf("could not ensure installer data directory permissions: %w", err)
	}

	// create subdirectories
	err = os.MkdirAll(paths.PackagesPath, 0755)
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
