// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentPythonRuntimePackage = hooks{
	postInstall: postInstallPythonRuntime,
	preRemove:   preRemovePythonRuntime,
}

const (
	pythonRuntimePackage = "datadog-agent-python-runtime"
	agentEmbeddedPath    = "/opt/datadog-agent/embedded"
)

var pythonRuntimePackagePermissions = file.Permissions{
	{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
}

func postInstallPythonRuntime(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_python_runtime")
	defer func() { span.Finish(err) }()

	if ctx.PackageType != PackageTypeOCI {
		return fmt.Errorf("unsupported package type for python runtime: %s", ctx.PackageType)
	}

	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to ensure dd-agent user and group: %v", err)
	}

	if err = pythonRuntimePackagePermissions.Ensure(ctx, ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set python runtime package permissions: %v", err)
	}

	srcEmbedded := filepath.Join(ctx.PackagePath, "embedded")
	if err = symlinkEmbeddedContents(srcEmbedded, agentEmbeddedPath); err != nil {
		return fmt.Errorf("failed to symlink python runtime into agent embedded directory: %v", err)
	}

	log.Infof("Python runtime installed from %s, symlinked into %s", ctx.PackagePath, agentEmbeddedPath)
	return nil
}

func preRemovePythonRuntime(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("remove_python_runtime")
	defer func() { span.Finish(err) }()

	srcEmbedded := filepath.Join(ctx.PackagePath, "embedded")
	if err = removeEmbeddedSymlinks(srcEmbedded, agentEmbeddedPath); err != nil {
		return fmt.Errorf("failed to remove python runtime symlinks: %v", err)
	}

	log.Infof("Python runtime symlinks removed from %s", agentEmbeddedPath)
	return nil
}

// symlinkEmbeddedContents creates symlinks from files in srcDir into dstDir.
// For top-level entries in srcDir (e.g. bin/, lib/), if the entry does not
// exist in dstDir it is symlinked directly. If a directory with the same name
// already exists in dstDir (e.g. agent's own embedded/lib/ for OpenSSL),
// the function recurses one level and symlinks individual files within it.
func symlinkEmbeddedContents(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %w", srcDir, err)
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		dstInfo, dstErr := os.Lstat(dstPath)
		if dstErr != nil && !os.IsNotExist(dstErr) {
			return fmt.Errorf("failed to stat %s: %w", dstPath, dstErr)
		}

		if os.IsNotExist(dstErr) {
			if err := os.Symlink(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to symlink %s -> %s: %w", dstPath, srcPath, err)
			}
			log.Debugf("Symlinked %s -> %s", dstPath, srcPath)
			continue
		}

		if dstInfo.IsDir() && entry.IsDir() {
			if err := symlinkEmbeddedContents(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		log.Debugf("Skipping %s: already exists in destination and is not a directory merge candidate", dstPath)
	}

	return nil
}

// removeEmbeddedSymlinks removes symlinks in dstDir that point into srcDir.
func removeEmbeddedSymlinks(srcDir, dstDir string) error {
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read directory %s: %w", dstDir, err)
	}

	for _, entry := range entries {
		dstPath := filepath.Join(dstDir, entry.Name())

		info, err := os.Lstat(dstPath)
		if err != nil {
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(dstPath)
			if err != nil {
				continue
			}
			if isUnderDir(target, srcDir) {
				if err := os.Remove(dstPath); err != nil {
					return fmt.Errorf("failed to remove symlink %s: %w", dstPath, err)
				}
				log.Debugf("Removed symlink %s -> %s", dstPath, target)
			}
		} else if info.IsDir() {
			if err := removeEmbeddedSymlinks(srcDir, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func isUnderDir(path, dir string) bool {
	absPath, err1 := filepath.Abs(path)
	absDir, err2 := filepath.Abs(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	return len(absPath) > len(absDir) && absPath[:len(absDir)+1] == absDir+string(filepath.Separator)
}
