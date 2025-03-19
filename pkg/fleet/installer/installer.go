// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer provides a package manager that installs and uninstalls packages.
package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"gopkg.in/yaml.v3"

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

	Install(ctx context.Context, url string, args []string) error
	ForceInstall(ctx context.Context, url string, args []string) error
	Remove(ctx context.Context, pkg string) error
	Purge(ctx context.Context)

	InstallExperiment(ctx context.Context, url string) error
	RemoveExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	InstallConfigExperiment(ctx context.Context, pkg string, version string, rawConfig []byte) error
	RemoveConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	GarbageCollect(ctx context.Context) error

	InstrumentAPMInjector(ctx context.Context, method string) error
	UninstrumentAPMInjector(ctx context.Context, method string) error

	Postinst(ctx context.Context, pkg string, caller string) error

	Close() error
}

// installerImpl is the implementation of the package manager.
type installerImpl struct {
	m sync.Mutex

	env        *env.Env
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
	i := &installerImpl{
		env:        env,
		db:         db,
		downloader: oci.NewDownloader(env, env.HTTPClient()),
		packages:   repository.NewRepositories(paths.PackagesPath, packages.PreRemoveHooks),
		configs:    repository.NewRepositories(paths.ConfigsPath, nil),

		userConfigsDir: paths.DefaultUserConfigsDir,
		packagesDir:    paths.PackagesPath,
	}

	err = i.ensurePackagesAreConfigured(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not ensure packages are configured: %w", err)
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
func (i *installerImpl) ConfigState(_ context.Context, pkg string) (repository.State, error) {
	return i.configs.GetState(pkg)
}

// ConfigStates returns the states of all packages.
func (i *installerImpl) ConfigStates(_ context.Context) (map[string]repository.State, error) {
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

func (i *installerImpl) doInstall(ctx context.Context, url string, args []string, shouldInstallPredicate func(dbPkg db.Package, pkg *oci.DownloadedPackage) bool) error {
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
	dbPkg, err := i.db.GetPackage(pkg.Name)
	if err != nil && !errors.Is(err, db.ErrPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	}
	if !shouldInstallPredicate(dbPkg, pkg) {
		return nil
	}
	err = i.preparePackage(ctx, pkg.Name, args) // Preinst
	if err != nil {
		return fmt.Errorf("could not prepare package: %w", err)
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
	err = i.packages.Create(ctx, pkg.Name, pkg.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	err = i.initPackageConfig(ctx, pkg.Name) // Config
	if err != nil {
		return fmt.Errorf("could not configure package: %w", err)
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
	err = repository.SetExperiment(ctx, pkg.Version, tmpDir)
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
		err := repository.DeleteExperiment(ctx)
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
		err = repository.DeleteExperiment(ctx)
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
	err := repository.PromoteExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	err = i.promoteExperiment(ctx, pkg)
	if err != nil {
		return err
	}

	// Update db
	state, err := repository.GetState()
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
func (i *installerImpl) InstallConfigExperiment(ctx context.Context, pkg string, version string, rawConfig []byte) error {
	i.m.Lock()
	defer i.m.Unlock()

	tmpDir, err := i.configs.MkdirTemp()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not create temporary directory: %w", err),
		)
	}
	defer os.RemoveAll(tmpDir)

	err = i.writeConfig(tmpDir, rawConfig)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not write agent config: %w", err),
		)
	}

	configRepo := i.configs.Get(pkg)
	err = configRepo.SetExperiment(ctx, version, tmpDir)
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
	err = repository.DeleteExperiment(ctx)
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
	err := repository.PromoteExperiment(ctx)
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
func (i *installerImpl) GarbageCollect(ctx context.Context) error {
	i.m.Lock()
	defer i.m.Unlock()
	err := i.packages.Cleanup(ctx)
	if err != nil {
		return fmt.Errorf("could not cleanup packages: %w", err)
	}
	err = i.configs.Cleanup(ctx)
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
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Postinst runs the post-install script for a package.
func (i *installerImpl) Postinst(ctx context.Context, pkg string, caller string) error {
	i.m.Lock()
	defer i.m.Unlock()

	if caller != "deb" && caller != "rpm" && caller != "oci" {
		return fmt.Errorf("invalid caller: %s", caller)
	}

	switch pkg {
	case packageDatadogAgent:
		installPath := filepath.Join(paths.PackagesPath, pkg, "stable")
		if caller == "deb" || caller == "rpm" {
			installPath = "/opt/datadog-agent"
		}
		return packages.PostInstallAgent(ctx, installPath, caller)
	default:
		return nil
	}
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
	case packageAPMLibraryDotnet:
		return packages.StartAPMLibraryDotnetExperiment(ctx)
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
	case packageAPMLibraryDotnet:
		return packages.StopAPMLibraryDotnetExperiment(ctx)
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
	case packageAPMLibraryDotnet:
		return packages.PromoteAPMLibraryDotnetExperiment(ctx)
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
	case packageAPMLibraryDotnet:
		return packages.SetupAPMLibraryDotnet(ctx, pkg)
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
	case packageAPMLibraryDotnet:
		return packages.RemoveAPMLibraryDotnet(ctx)
	default:
		return nil
	}
}

func (i *installerImpl) ensurePackagesAreConfigured(ctx context.Context) (err error) {
	pkgList, err := i.packages.GetStates()
	if err != nil {
		return fmt.Errorf("could not get package states: %w", err)
	}
	for pkg := range pkgList {
		err = i.initPackageConfig(ctx, pkg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *installerImpl) initPackageConfig(ctx context.Context, pkg string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "configure_package")
	defer func() { span.Finish(err) }()
	// TODO: Windows support
	if runtime.GOOS == "windows" {
		return nil
	}
	state, err := i.configs.GetState(pkg)
	if err != nil {
		return fmt.Errorf("could not get config repository state: %w", err)
	}
	// If a config is already set, no need to initialize it
	if state.Stable != "" {
		return nil
	}
	tmpDir, err := i.configs.MkdirTemp()
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = i.configs.Create(ctx, pkg, "empty", tmpDir)
	if err != nil {
		return fmt.Errorf("could not create %s repository: %w", pkg, err)
	}
	return nil
}

var (
	allowedConfigFiles = []string{
		"/datadog.yaml",
		"/security-agent.yaml",
		"/system-probe.yaml",
		"/application_monitoring.yaml",
		"/conf.d/*.yaml",
		"/conf.d/*.d/*.yaml",
	}
)

func configNameAllowed(file string) bool {
	for _, allowedFile := range allowedConfigFiles {
		match, err := filepath.Match(allowedFile, file)
		if err != nil {
			return false
		}
		if match {
			return true
		}
	}
	return false
}

type configFile struct {
	Path     string          `json:"path"`
	Contents json.RawMessage `json:"contents"`
}

func (i *installerImpl) writeConfig(dir string, rawConfig []byte) error {
	var files []configFile
	err := json.Unmarshal(rawConfig, &files)
	if err != nil {
		return fmt.Errorf("could not unmarshal config files: %w", err)
	}
	for _, file := range files {
		file.Path = filepath.Clean(file.Path)
		if !configNameAllowed(file.Path) {
			return fmt.Errorf("config file %s is not allowed", file)
		}
		var c interface{}
		err = json.Unmarshal(file.Contents, &c)
		if err != nil {
			return fmt.Errorf("could not unmarshal config file contents: %w", err)
		}
		serialized, err := yaml.Marshal(c)
		if err != nil {
			return fmt.Errorf("could not serialize config file contents: %w", err)
		}
		err = os.MkdirAll(filepath.Join(dir, filepath.Dir(file.Path)), 0755)
		if err != nil {
			return fmt.Errorf("could not create config file directory: %w", err)
		}
		err = os.WriteFile(filepath.Join(dir, file.Path), serialized, 0644)
		if err != nil {
			return fmt.Errorf("could not write config file: %w", err)
		}
	}
	return nil
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
	return nil
}
