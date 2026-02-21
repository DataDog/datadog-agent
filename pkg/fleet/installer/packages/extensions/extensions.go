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

// ExtensionsDBDir is the path to the extensions database, overridden in tests
var ExtensionsDBDir = paths.RunPath

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

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	return db.RemovePackage(pkg, isExperiment)
}

// Install installs extensions for a package.
func Install(ctx context.Context, downloader *oci.Downloader, url string, extensions []string, isExperiment bool, hooks ExtensionHooks) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.install")
	defer func() { span.Finish(err) }()
	span.SetTag("extensions", strings.Join(extensions, ","))
	span.SetTag("url", url)
	span.SetTag("is_experiment", isExperiment)

	// Open & lock the extensions database
	db, err := newExtensionsDB(filepath.Join(ExtensionsDBDir, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	// Download package metadata
	pkg, err := downloader.Download(ctx, url)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	span.SetTag("package_name", pkg.Name)
	span.SetTag("package_version", pkg.Version)

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
		dbPkg.Extensions = make(map[string]struct{})
	}

	// Process each extension
	for _, extension := range extensions {
		// Check if extension is already installed with the same package version
		if _, exists := dbPkg.Extensions[extension]; exists {
			fmt.Printf("Extension %s already installed, skipping\n", extension)
			continue
		}

		err := installSingle(ctx, pkg, extension, isExperiment, hooks)
		if err != nil {
			fmt.Printf("Failed to install extension %s: %v\n", extension, err)
			continue
		}

		// Mark as installed
		dbPkg.Extensions[extension] = struct{}{}
	}

	// Update DB now that all extensions installed successfully
	err = db.SetPackage(dbPkg, isExperiment)
	if err != nil {
		return fmt.Errorf("could not update package in db: %w", err)
	}

	return nil
}

// installSingle installs a single extension for a package.
func installSingle(ctx context.Context, pkg *oci.DownloadedPackage, extension string, isExperiment bool, hooks ExtensionHooks) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "extensions.install_single")
	defer func() { span.Finish(err) }()
	span.SetTag("extension", extension)
	span.SetTag("package_name", pkg.Name)
	span.SetTag("package_version", pkg.Version)

	// TODO: Remove previous extension if it exists

	// Pre-install hook
	err = hooks.PreInstallExtension(ctx, pkg.Name, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	// Extract to a temporary directory first
	tmpDir, err := os.MkdirTemp(paths.PackagesPath, pkg.Name+"-extension-")
	if err != nil {
		return fmt.Errorf("could not create temp directory for %s: %w", extension, err)
	}
	defer os.RemoveAll(tmpDir)

	err = pkg.ExtractLayers(oci.DatadogPackageExtensionLayerMediaType, tmpDir, oci.LayerAnnotation{Key: "com.datadoghq.package.extension.name", Value: extension})
	if err != nil {
		if errors.Is(err, oci.ErrNoLayerMatchesAnnotations) {
			// The extension is not available in the package, skip it.
			// This might be a version where the extension doesn't exist and shouldn't block other methods.
			fmt.Printf("no layer matches the requested annotations for %s, skipping\n", extension)
			return nil
		}
		return fmt.Errorf("could not extract layers for %s: %w", extension, err)
	}

	// Move to the final location
	extensionsPath := getExtensionsPath(pkg.Name, pkg.Version)
	if err := os.MkdirAll(extensionsPath, 0755); err != nil {
		return fmt.Errorf("could not create directory for %s: %w", extension, err)
	}
	extensionPath := filepath.Join(extensionsPath, extension)

	// Track whether we've moved files to final location
	moved := false

	// Defer cleanup of final location if we fail after moving
	defer func() {
		if err != nil && moved {
			log.Warnf("Installation failed for %s, cleaning up files at %s", extension, extensionPath)
			if cleanupErr := os.RemoveAll(extensionPath); cleanupErr != nil {
				log.Errorf("Failed to cleanup extension files at %s: %v", extensionPath, cleanupErr)
				// Add cleanup error to the returned error
				err = fmt.Errorf("%w; cleanup failed: %v", err, cleanupErr)
			}
		}
	}()

	err = os.Rename(tmpDir, extensionPath)
	if err != nil {
		return fmt.Errorf("could not move %s to final location: %w", extension, err)
	}
	moved = true // Track that files are now in final location

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
		return fmt.Errorf("package %s is not installed, cannot save extensions", pkg)
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
func Restore(ctx context.Context, downloader *oci.Downloader, pkg string, downloadURL string, saveDir string, isExperiment bool, hooks ExtensionHooks) (err error) {
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
	extensions := strings.Split(content, "\n")
	span.SetTag("extensions", strings.Join(extensions, ","))

	err = Install(ctx, downloader, downloadURL, extensions, isExperiment, hooks)
	if err != nil {
		return fmt.Errorf("could not install extensions: %w", err)
	}

	return nil
}

// getExtensionsPath returns the path to the extensions for a package.
// For OCI-installed agents, extensions live under the OCI packages directory.
// For DEB/RPM-installed agents on Linux, the OCI directory doesn't exist, so we fall back to /opt/datadog-agent.
func getExtensionsPath(pkg, version string) string {
	basePath := filepath.Join(paths.PackagesPath, pkg, version)
	if pkg == "datadog-agent" && runtime.GOOS == "linux" {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			basePath = "/opt/datadog-agent"
		}
	}
	return filepath.Join(basePath, "ext")
}

// keys returns the keys of a map[string]struct{}.
func keys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
