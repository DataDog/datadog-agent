// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// IntegrationDepsPackageName is the OCI package that carries the third-party
// python dependencies of integrations.
const IntegrationDepsPackageName = "datadog-agent-integration-deps"

//nolint:unused // registered by packages_linux.go (linux-only for now)
var integrationDepsPackage = hooks{
	postInstall: postInstallIntegrationDeps,
	preRemove:   preRemoveIntegrationDeps,
}

// postInstallIntegrationDeps wires the dependency package's site-packages
// entries into every installed datadog-agent tree, so the running interpreter
// picks them up from its existing sys.path.
//
//nolint:unused // registered by packages_linux.go (linux-only for now)
func postInstallIntegrationDeps(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("install_integration_deps")
	defer func() { span.Finish(err) }()
	return wireIntegrationDeps(ctx, ctx.PackagePath)
}

// preRemoveIntegrationDeps removes the site-packages symlinks pointing into
// the dependency package.
//
//nolint:unused // registered by packages_linux.go (linux-only for now)
func preRemoveIntegrationDeps(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("remove_integration_deps")
	defer func() { span.Finish(err) }()
	return unwireIntegrationDeps(ctx, ctx.PackagePath)
}

// rewireIntegrationDeps re-creates the dependency package's site-packages
// symlinks in the (new) datadog-agent trees. It is called from the agent
// package hooks because the agent's site-packages directory is replaced
// wholesale on every agent install or upgrade.
//
//nolint:unused // called from the linux agent package hooks
func rewireIntegrationDeps(ctx HookContext) error {
	return wireIntegrationDepsIn(ctx, paths.PackagesPath, paths.PackagesPath)
}

// wireIntegrationDepsIn wires the dependency package installed at
// packagesRoot/<IntegrationDepsPackageName>/stable into every datadog-agent
// tree installed under packagesRoot.
func wireIntegrationDepsIn(ctx HookContext, packagesRoot, depsRoot string) error {
	depsPackagePath, err := filepath.EvalSymlinks(filepath.Join(depsRoot, IntegrationDepsPackageName, "stable"))
	if err != nil {
		return nil
	}
	return wireIntegrationDepsAt(ctx, packagesRoot, depsPackagePath)
}

// sitePackagesDirsIn returns the site-packages directories of the python
// tree shipped at the given package path (e.g.
// <package>/embedded/lib/python3.12/site-packages).
func sitePackagesDirsIn(packagePath string) ([]string, error) {
	root, err := filepath.EvalSymlinks(packagePath)
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(root, "embedded", "lib", "python*", "site-packages"))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no site-packages directory found under %s", root)
	}
	return matches, nil
}

// agentSitePackagesDirs returns the site-packages directories of every
// datadog-agent tree (stable and experiment) installed under packagesRoot.
// The dependency package is only delivered through the fleet installer, so
// there is no wiring into package-manager agent installs.
func agentSitePackagesDirs(packagesRoot string) []string {
	dirs := []string{}
	for _, link := range []string{"stable", "experiment"} {
		agentRoot, err := filepath.EvalSymlinks(filepath.Join(packagesRoot, "datadog-agent", link))
		if err != nil {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(agentRoot, "embedded", "lib", "python*", "site-packages"))
		if err != nil {
			continue
		}
		dirs = append(dirs, matches...)
	}
	return dirs
}

//nolint:unused // linux-only registration
func wireIntegrationDeps(ctx HookContext, packagePath string) error {
	return wireIntegrationDepsAt(ctx, paths.PackagesPath, packagePath)
}

func wireIntegrationDepsAt(ctx HookContext, packagesRoot, packagePath string) error {
	sourceDirs, err := sitePackagesDirsIn(packagePath)
	if err != nil {
		return err
	}
	targetDirs := agentSitePackagesDirs(packagesRoot)
	if len(targetDirs) == 0 {
		return errors.New("no datadog-agent installation found to wire dependencies into")
	}
	for _, sourceDir := range sourceDirs {
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			for _, targetDir := range targetDirs {
				targetPath := filepath.Join(targetDir, entry.Name())
				// never replace real files, only refresh existing links
				if fi, err := os.Lstat(targetPath); err == nil && fi.Mode()&os.ModeSymlink == 0 {
					continue
				}
				if err := file.EnsureSymlink(ctx, filepath.Join(sourceDir, entry.Name()), targetPath); err != nil {
					return fmt.Errorf("could not wire dependency %s: %w", entry.Name(), err)
				}
			}
		}
	}
	return nil
}

//nolint:unused // linux-only registration
func unwireIntegrationDeps(ctx HookContext, packagePath string) error {
	return unwireIntegrationDepsAt(ctx, paths.PackagesPath, packagePath)
}

func unwireIntegrationDepsAt(ctx HookContext, packagesRoot, packagePath string) error {
	sourceDirs, err := sitePackagesDirsIn(packagePath)
	if err != nil {
		return err
	}
	targetDirs := agentSitePackagesDirs(packagesRoot)
	for _, sourceDir := range sourceDirs {
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			for _, targetDir := range targetDirs {
				targetPath := filepath.Join(targetDir, entry.Name())
				// only remove our symlinks, never real files
				if fi, err := os.Lstat(targetPath); err == nil && fi.Mode()&os.ModeSymlink == 0 {
					continue
				}
				if err := file.EnsureSymlinkAbsent(ctx, targetPath); err != nil {
					return fmt.Errorf("could not unwire dependency %s: %w", entry.Name(), err)
				}
			}
		}
	}
	return nil
}
