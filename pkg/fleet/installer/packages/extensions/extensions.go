// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package extensions contains the install/upgrades/uninstall logic for extensions
package extensions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errExtensionNotInPackage is returned by installSingle when the requested extension
// layer is absent from the package image.
var errExtensionNotInPackage = errors.New("extension not in package")

// ExtensionsDBDir is the path to the extensions database, overridden in tests
var ExtensionsDBDir = paths.RunPath

// ExtensionsPackagesPath is the base directory for extension files, overridden in tests
var ExtensionsPackagesPath = paths.PackagesPath

// ExtensionRegistry holds per-extension registry override settings.
type ExtensionRegistry struct {
	URL      string
	Auth     string
	Username string
	Password string
}

// ExtensionHooks is the interface for the extension hooks.
type ExtensionHooks interface {
	PreInstallExtension(ctx context.Context, pkg string, extension string) error
	PreRemoveExtension(ctx context.Context, pkg string, extension string) error
	PostInstallExtension(ctx context.Context, pkg string, extension string, isExperiment bool) error
}

// SetPackage sets the version of a package in the database.
func SetPackage(ctx context.Context, pkg string, version string, isExperiment bool) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "extensions.set_package")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)
	span.SetTag("package_version", version)
	span.SetTag("is_experiment", isExperiment)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	return db.SetPackageVersion(pkg, version, isExperiment)
}

// DeletePackage removes a package from the database.
func DeletePackage(ctx context.Context, pkg string, isExperiment bool) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "extensions.delete_package")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)
	span.SetTag("is_experiment", isExperiment)

	dbPath := filepath.Join(ExtensionsDBDir, "extensions.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}

	// Open & lock the extensions database
	db, err := newExtensionsDB(dbPath)
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	return db.RemovePackage(pkg, isExperiment)
}

// Install installs extensions for a package.
// If overrides is non-nil, extensions whose name appears as a key will be
// downloaded from the corresponding registry instead of the default one.
func Install(ctx context.Context, downloader *oci.Downloader, url string, extensionsList []string, isExperiment bool, hooks ExtensionHooks, overrides map[string]ExtensionRegistry) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.install")
	defer func() { span.Finish(err) }()
	span.SetTag("extensions", strings.Join(extensionsList, ","))
	span.SetTag("url", url)
	span.SetTag("is_experiment", isExperiment)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Group extensions by their effective downloader (default vs override).
	type downloaderGroup struct {
		downloader *oci.Downloader
		extensions []string
	}
	defaultGroup := &downloaderGroup{downloader: downloader}
	overrideGroups := map[string]*downloaderGroup{} // keyed by override URL for dedup

	for _, ext := range extensionsList {
		if o, ok := overrides[ext]; ok && o.URL != "" {
			key := o.URL
			g, exists := overrideGroups[key]
			if !exists {
				g = &downloaderGroup{
					downloader: downloader.WithRegistryOverride(o.URL, o.Auth, o.Username, o.Password),
				}
				overrideGroups[key] = g
			}
			g.extensions = append(g.extensions, ext)
		} else {
			defaultGroup.extensions = append(defaultGroup.extensions, ext)
		}
	}

	groups := []*downloaderGroup{}
	if len(defaultGroup.extensions) > 0 {
		groups = append(groups, defaultGroup)
	}
	for _, g := range overrideGroups {
		groups = append(groups, g)
	}

	var installErrors []error
	tagSet := false
	for _, group := range groups {
		// Download package metadata once per distinct registry
		pkg, err := group.downloader.Download(ctx, url)
		if err != nil {
			installErrors = append(installErrors, installerErrors.Wrap(
				installerErrors.ErrDownloadFailed,
				fmt.Errorf("could not download package: %w", err),
			))
			continue
		}
		if !tagSet {
			span.SetTag("package_name", pkg.Name)
			span.SetTag("package_version", pkg.Version)
			tagSet = true
		}

		// Check if package is already installed
		dbPkg, err := db.GetPackage(pkg.Name, isExperiment)
		if err != nil && !errors.Is(err, errPackageNotFound) {
			return fmt.Errorf("could not check if package %s is installed: %w", pkg.Name, err)
		} else if err != nil && errors.Is(err, errPackageNotFound) {
			return fmt.Errorf("package %s is not installed", pkg.Name)
		} else if dbPkg.Version != pkg.Version {
			return fmt.Errorf("package %s is installed at version %s, requested version is %s", pkg.Name, dbPkg.Version, pkg.Version)
		}
		if dbPkg.Extensions == nil {
			dbPkg.Extensions = make(map[string]string)
		}

		newDigest, err := pkg.Image.Digest()
		if err != nil {
			return fmt.Errorf("could not get digest for package %s: %w", pkg.Name, err)
		}
		newDigestStr := newDigest.String()

		// Process each extension in this group
		for _, extension := range group.extensions {
			stored, exists := dbPkg.Extensions[extension]
			if exists && stored == newDigestStr {
				log.Debugf("Extension %s already installed at digest %s, skipping", extension, stored)
				continue
			}

			err := installSingle(ctx, pkg, extension, isExperiment, hooks, exists)
			if err != nil {
				if !errors.Is(err, errExtensionNotInPackage) {
					installErrors = append(installErrors, fmt.Errorf("extension %s: %w", extension, err))
				}
				continue
			}

			dbPkg.Extensions[extension] = newDigestStr
		}

		// Update DB with successfully installed extensions
		err = db.SetPackage(dbPkg, isExperiment)
		if err != nil {
			return fmt.Errorf("could not update package in db: %w", err)
		}
	}

	return errors.Join(installErrors...)
}

// installSingle installs a single extension for a package.
func installSingle(ctx context.Context, pkg *oci.DownloadedPackage, extension string, isExperiment bool, hooks ExtensionHooks, replace bool) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.install_single")
	defer func() { span.Finish(err) }()
	span.SetTag("extension", extension)
	span.SetTag("package_name", pkg.Name)
	span.SetTag("package_version", pkg.Version)

	extensionsPath := getExtensionsPath(pkg.Name, pkg.Version)
	extensionPath := filepath.Join(extensionsPath, extension)

	if _, statErr := os.Stat(extensionPath); statErr == nil && !replace {
		log.Debugf("Extension %s already installed and replace is disabled, skipping", extension)
		return nil
	}

	// Extract to a temporary directory first
	tmpDir, err := os.MkdirTemp(ExtensionsPackagesPath, pkg.Name+"-extension-")
	if err != nil {
		return fmt.Errorf("could not create temp directory for %s: %w", extension, err)
	}
	defer os.RemoveAll(tmpDir)

	err = pkg.ExtractLayers(ctx, oci.DatadogPackageExtensionLayerMediaType, tmpDir, oci.LayerAnnotation{Key: "com.datadoghq.package.extension.name", Value: extension})
	if err != nil {
		if errors.Is(err, oci.ErrNoLayerMatchesAnnotations) {
			// The extension is not available in the package, skip it.
			// This might be a version where the extension doesn't exist and shouldn't block other methods.
			fmt.Printf("no layer matches the requested annotations for %s, skipping\n", extension)
			return errExtensionNotInPackage
		}
		return fmt.Errorf("could not extract layers for %s: %w", extension, err)
	}

	// Move to the final location
	if err := os.MkdirAll(extensionsPath, 0755); err != nil {
		return fmt.Errorf("could not create directory for %s: %w", extension, err)
	}

	// moved tracks whether new content has been placed at extensionPath.
	// backupDir, when non-empty, holds the previous installation for rollback.
	moved := false
	var backupDir string

	// On failure: remove new (partial) content, then restore the previous installation.
	defer func() {
		if err != nil {
			if moved {
				log.Warnf("Installation failed for %s, cleaning up files at %s", extension, extensionPath)
				if cleanupErr := os.RemoveAll(extensionPath); cleanupErr != nil {
					log.Errorf("Failed to cleanup extension files at %s: %v", extensionPath, cleanupErr)
					err = fmt.Errorf("%w; cleanup failed: %v", err, cleanupErr)
				}
			}
			if backupDir != "" {
				if hookErr := hooks.PreInstallExtension(ctx, pkg.Name, extension); hookErr != nil {
					log.Errorf("Failed pre-install hook during rollback for extension %s: %v", extension, hookErr)
				}
				if renameErr := paths.Rename(ctx, backupDir, extensionPath); renameErr != nil {
					log.Errorf("Failed to restore previous extension %s from %s: %v", extension, backupDir, renameErr)
				} else if hookErr := hooks.PostInstallExtension(ctx, pkg.Name, extension, isExperiment); hookErr != nil {
					log.Errorf("Failed post-install hook during rollback for extension %s: %v", extension, hookErr)
				}
			}
		}
		if backupDir != "" {
			os.RemoveAll(backupDir)
		}
	}()

	if replace {
		// Copy the current installation to a backup and remove previous installation
		backupDir, err = os.MkdirTemp(filepath.Dir(extensionPath), extension+"-backup-")
		if err != nil {
			return fmt.Errorf("could not create backup directory for extension %s: %w", extension, err)
		}
		if err = os.CopyFS(backupDir, os.DirFS(extensionPath)); err != nil {
			os.RemoveAll(backupDir)
			backupDir = "" // no backup; prevent restore attempt in defer
			return fmt.Errorf("could not copy extension %s to backup: %w", extension, err)
		}
		if err = removeSingle(ctx, pkg.Name, pkg.Version, extension, hooks); err != nil {
			return fmt.Errorf("could not remove existing extension %s: %w", extension, err)
		}
	}

	// Pre-install hook
	if err = hooks.PreInstallExtension(ctx, pkg.Name, extension); err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	err = paths.Rename(ctx, tmpDir, extensionPath)
	if err != nil {
		return fmt.Errorf("could not move %s to final location: %w", extension, err)
	}
	moved = true

	if err := os.Chmod(extensionPath, 0755); err != nil {
		return fmt.Errorf("could not set permissions on extension directory %s: %w", extensionPath, err)
	}

	// Post-install hook
	err = hooks.PostInstallExtension(ctx, pkg.Name, extension, isExperiment)
	if err != nil {
		return fmt.Errorf("could not install extension: %w", err)
	}

	return nil
}

// Remove removes extensions for a package.
func Remove(ctx context.Context, pkg string, extensions []string, isExperiment bool, hooks ExtensionHooks) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.remove")
	defer func() { span.Finish(err) }()
	span.SetTag("extensions", strings.Join(extensions, ","))
	span.SetTag("package_name", pkg)
	span.SetTag("is_experiment", isExperiment)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Get package from database
	dbPkg, err := db.GetPackage(pkg, isExperiment)
	if err != nil && !errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("could not get package %s: %w", pkg, err)
	} else if err != nil && errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("package %s is not installed, cannot remove extensions", pkg)
	}

	// Process each extension
	var removeErrors []error
	for _, extension := range extensions {
		// Check if extension is installed
		if _, exists := dbPkg.Extensions[extension]; !exists {
			fmt.Printf("Extension %s is not installed, skipping\n", extension)
			continue
		}

		err := removeSingle(ctx, pkg, dbPkg.Version, extension, hooks)
		if err != nil {
			removeErrors = append(removeErrors, err)
			continue
		}

		// Mark as removed
		delete(dbPkg.Extensions, extension)
	}

	err = db.SetPackage(dbPkg, isExperiment)
	if err != nil {
		return fmt.Errorf("could not update package in db: %w", err)
	}

	return errors.Join(removeErrors...)
}

// RemoveAll removes all extensions for a package.
func RemoveAll(ctx context.Context, pkg string, isExperiment bool, hooks ExtensionHooks) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.remove_all")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)

	dbPath := filepath.Join(ExtensionsDBDir, "extensions.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}

	// Open & lock the extensions database
	db, err := newExtensionsDB(dbPath)
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Get package from database
	dbPkg, err := db.GetPackage(pkg, isExperiment)
	if err != nil && !errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("could not get package %s: %w", pkg, err)
	} else if err != nil && errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("package %s is not installed, cannot remove all extensions", pkg)
	}

	// Process each extension
	var removeErrors []error
	for extension := range dbPkg.Extensions {
		err := removeSingle(ctx, pkg, dbPkg.Version, extension, hooks)
		if err != nil {
			removeErrors = append(removeErrors, err)
			continue
		}

		// Mark as removed
		delete(dbPkg.Extensions, extension)
	}

	err = db.SetPackage(dbPkg, isExperiment)
	if err != nil {
		return fmt.Errorf("could not update package in db: %w", err)
	}

	return errors.Join(removeErrors...)
}

// removeSingle removes a single extension for a package.
func removeSingle(ctx context.Context, pkg string, version string, extension string, hooks ExtensionHooks) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.remove_single")
	defer func() { span.Finish(err) }()
	span.SetTag("extension", extension)
	span.SetTag("package_name", pkg)
	span.SetTag("package_version", version)

	// Pre-remove hook
	err = hooks.PreRemoveExtension(ctx, pkg, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	// Remove the extension
	extensionsPath := getExtensionsPath(pkg, version)
	extensionPath := filepath.Join(extensionsPath, extension)
	err = os.RemoveAll(extensionPath)
	if err != nil {
		return fmt.Errorf("could not remove extension: %w", err)
	}

	return nil
}

// Promote promotes a package's extensions from experiment to stable.
func Promote(ctx context.Context, pkg string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "extensions.promote")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Promote the extensions
	err = db.PromotePackage(pkg)
	if err != nil {
		return fmt.Errorf("could not promote extensions: %w", err)
	}

	return nil
}

// Save saves the extensions for a package upgrade
func Save(ctx context.Context, pkg string, saveDir string, isExperiment bool) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "extensions.save")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)
	span.SetTag("save_dir", saveDir)
	span.SetTag("is_experiment", isExperiment)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Get package from database
	dbPkg, err := db.GetPackage(pkg, isExperiment)
	if err != nil && !errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("could not get package %s from db: %w", pkg, err)
	} else if err != nil && errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("package %s is not installed, cannot save extensions: %w", pkg, errPackageNotFound)
	}

	if len(dbPkg.Extensions) == 0 {
		return nil // No extensions to save
	}
	span.SetTag("extensions", strings.Join(keys(dbPkg.Extensions), ","))

	savePath := filepath.Join(saveDir, fmt.Sprintf(".%s-extensions.txt", pkg))
	err = os.WriteFile(savePath, []byte(strings.Join(keys(dbPkg.Extensions), "\n")), 0644)
	if err != nil {
		return fmt.Errorf("could not write extensions to file: %w", err)
	}

	return nil
}

// Restore restores the extensions after a package upgrade
func Restore(ctx context.Context, downloader *oci.Downloader, pkg string, downloadURL string, saveDir string, isExperiment bool, hooks ExtensionHooks, overrides map[string]ExtensionRegistry) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.restore")
	defer func() { span.Finish(err) }()
	span.SetTag("package_name", pkg)
	span.SetTag("download_url", downloadURL)
	span.SetTag("save_dir", saveDir)
	span.SetTag("is_experiment", isExperiment)

	savePath := filepath.Join(saveDir, fmt.Sprintf(".%s-extensions.txt", pkg))
	f, err := os.ReadFile(savePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read extensions file: %w", err)
	}
	content := strings.TrimSpace(string(f))
	if content == "" {
		return nil // No extensions to restore
	}
	extensionsList := strings.Split(content, "\n")
	span.SetTag("extensions", strings.Join(extensionsList, ","))

	err = Install(ctx, downloader, downloadURL, extensionsList, isExperiment, hooks, overrides)
	if err != nil {
		return fmt.Errorf("could not install extensions: %w", err)
	}

	// On non-Windows platforms the save file lives in a transient tmp directory and should
	// be cleaned up after a successful restore to avoid phantom-restoring stale extensions
	// on the next upgrade. On Windows, the save file is kept in ProtectedDir so that it
	// survives the MSI upgrade process (it is only written once, before prerm).
	if runtime.GOOS != "windows" {
		if err := os.Remove(savePath); err != nil && !os.IsNotExist(err) {
			log.Warnf("could not remove extension save file %s: %v", savePath, err)
		}
	}

	return nil
}

// getExtensionsPath returns the path to the extensions for a package.
// For OCI-installed agents, extensions live under the OCI packages directory.
// For DEB/RPM-installed agents on Linux, the OCI directory doesn't exist, so we fall back to /opt/datadog-agent.
func getExtensionsPath(pkg, version string) string {
	basePath := filepath.Join(ExtensionsPackagesPath, pkg, version)
	if pkg == "datadog-agent" && runtime.GOOS == "linux" {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			basePath = "/opt/datadog-agent"
		}
	}
	return filepath.Join(basePath, "ext")
}

// keys returns the keys of a map[string]string.
func keys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
