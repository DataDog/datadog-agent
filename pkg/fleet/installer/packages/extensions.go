// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ListExtensions lists all extensions for a package.
// Returns all extensions for a package, empty if the package is not installed.
func ListExtensions(pkg string, experiment bool) ([]string, error) {
	db, err := newExtensionsDB(filepath.Join(paths.RunPath, "extensions.db"))
	if err != nil {
		return nil, fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	dbPkg, err := db.GetPackage(pkg, experiment)
	if err != nil {
		if errors.Is(err, errPackageNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not get package: %w", err)
	}

	extensions := make([]string, 0, len(dbPkg.Extensions))
	for ext := range dbPkg.Extensions {
		extensions = append(extensions, ext)
	}
	return extensions, nil
}

// InstallExtensions installs multiple extensions for a package.
func InstallExtensions(ctx context.Context, pkg *oci.DownloadedPackage, extensions []string, experiment bool, hooks Hooks) error {
	db, err := newExtensionsDB(filepath.Join(paths.RunPath, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	dbPkg, err := db.GetPackage(pkg.Name, experiment)
	if err != nil && !errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	} else if err != nil && errors.Is(err, errPackageNotFound) {
		dbPkg = dbPackage{
			Name:       pkg.Name,
			Version:    pkg.Version,
			Extensions: map[string]struct{}{},
		}
	} else if dbPkg.Version != pkg.Version {
		return fmt.Errorf("package %s is installed at version %s, requested version is %s", pkg.Name, dbPkg.Version, pkg.Version)
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
		// Check if extension is already installed with the same package version
		if _, exists := dbPkg.Extensions[extension]; exists && dbPkg.Version == pkg.Version {
			log.Infof("Extension %s already installed, skipping", extension)
			fmt.Printf("Extension %s already installed, skipping\n", extension)
			continue
		}

		err := installExtension(ctx, pkg, extension, hooks)
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
		err = db.SetPackage(dbPkg, experiment)
		if err != nil {
			// Clean up on failure
			for _, extension := range installedExtensions {
				extractDir := filepath.Join(paths.PackagesPath, pkg.Name, pkg.Version, "ext", extension)
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
func installExtension(ctx context.Context, pkg *oci.DownloadedPackage, extension string, hooks Hooks) error {
	err := hooks.PreInstallExtension(ctx, pkg.Name, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	// Extract to a temporary directory first
	// tmpDir, err := i.packages.MkdirTemp()
	tmpDir, err := os.MkdirTemp("", "dd-extension-") // TODO fix this
	if err != nil {
		return fmt.Errorf("could not create temp directory for %s: %w", extension, err)
	}
	defer os.RemoveAll(tmpDir)

	err = pkg.ExtractLayers(oci.DatadogPackageExtensionLayerMediaType, tmpDir, oci.LayerAnnotation{Key: "com.datadoghq.package.extension.name", Value: extension})
	if err != nil {
		if errors.Is(err, oci.ErrNoLayerMatchesAnnotations) {
			log.Warnf("no layer matches the requested annotations for %s", extension)
			return nil // The extension is not available in the package, skip it. Might be an incompatible version, but this shouldn't block other methods.
		}
		return fmt.Errorf("could not extract layers for %s: %w", extension, err)
	}

	extractDir := filepath.Join(paths.PackagesPath, pkg.Name, pkg.Version, "ext", extension)
	if err := os.MkdirAll(filepath.Dir(extractDir), 0755); err != nil {
		return fmt.Errorf("could not create directory for %s: %w", extension, err)
	}

	err = os.Rename(tmpDir, extractDir)
	if err != nil {
		return fmt.Errorf("could not move %s to final location: %w", extension, err)
	}

	err = hooks.PostInstallExtension(ctx, pkg.Name, extension)
	if err != nil {
		return fmt.Errorf("could not install extension: %w", err)
	}
	return nil
}

// RemoveExtensions removes multiple extensions.
func RemoveExtensions(ctx context.Context, pkg string, extensions []string, experiment bool, hooks Hooks) error {
	db, err := newExtensionsDB(filepath.Join(paths.RunPath, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	if len(extensions) == 0 {
		return nil
	}

	dbPkg, err := db.GetPackage(pkg, experiment)
	if err != nil && !errors.Is(err, errPackageNotFound) {
		return fmt.Errorf("could not get package: %w", err)
	} else if err != nil && errors.Is(err, errPackageNotFound) {
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

		err := removeExtension(ctx, pkg, dbPkg.Version, extension, hooks)
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
		err = db.SetPackage(dbPkg, experiment)
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

// RemoveAllExtensions removes all extensions for a package, called when the package is being removed.
func RemoveAllExtensions(ctx context.Context, pkg string, hooks Hooks) error {
	db, err := newExtensionsDB(filepath.Join(paths.RunPath, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	dbPkg, err := db.GetPackage(pkg, false)
	if err != nil {
		if !errors.Is(err, errPackageNotFound) {
			return fmt.Errorf("could not get package: %w", err)
		}
	}

	for extension := range dbPkg.Extensions {
		if err := removeExtension(ctx, pkg, dbPkg.Version, extension, hooks); err != nil {
			log.Warnf("Failed to remove extension %s from package %s: %v", extension, pkg, err)
		}
	}

	err = db.RemovePackage(pkg, false)
	if err != nil {
		return fmt.Errorf("could not remove package from extensions db: %w", err)
	}

	return nil
}

// removeExtension removes a single extension for a package.
func removeExtension(ctx context.Context, pkg, version, extension string, hooks Hooks) error {
	err := hooks.PreRemoveExtension(ctx, pkg, extension)
	if err != nil {
		return fmt.Errorf("could not prepare extension: %w", err)
	}

	extensionDir := filepath.Join(paths.PackagesPath, pkg, version, "ext", extension)
	err = os.RemoveAll(extensionDir)
	if err != nil {
		return fmt.Errorf("could not remove directory for %s: %w", extension, err)
	}
	return nil
}

// PromoteExtensions promotes an extension from experiment to stable. As the installer handles the top-level dir,
// we only have to update the DB.
func PromoteExtensions(pkg string) error {
	db, err := newExtensionsDB(filepath.Join(paths.RunPath, "extensions.db"))
	if err != nil {
		return fmt.Errorf("could not create extensions db: %w", err)
	}
	defer db.Close()

	err = db.Promote(pkg)
	if err != nil {
		return fmt.Errorf("could not promote extensions: %w", err)
	}
	return nil
}
