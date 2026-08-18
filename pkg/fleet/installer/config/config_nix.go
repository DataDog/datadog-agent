// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	deploymentIDFile = ".deployment-id"
)

// GetState returns the state of the directories.
func (d *Directories) GetState() (State, error) {
	stablePath := filepath.Join(d.StablePath, deploymentIDFile)
	experimentPath := filepath.Join(d.ExperimentPath, deploymentIDFile)
	stableDeploymentID, err := os.ReadFile(stablePath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	experimentDeploymentID, err := os.ReadFile(experimentPath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	stableExists := len(stableDeploymentID) > 0
	experimentExists := len(experimentDeploymentID) > 0
	// If experiment is symlinked to stable, it means the experiment is not installed.
	if stableExists && experimentExists && isSameFile(stablePath, experimentPath) {
		experimentDeploymentID = nil
	}
	return State{
		StableDeploymentID:     string(stableDeploymentID),
		ExperimentDeploymentID: string(experimentDeploymentID),
	}, nil
}

// WriteExperiment writes the experiment to the directories.
func (d *Directories) WriteExperiment(ctx context.Context, operations Operations) error {
	err := os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = copyDirectory(ctx, d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}

	operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)

	err = operations.Apply(ctx, d.ExperimentPath)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(d.ExperimentPath, deploymentIDFile), []byte(operations.DeploymentID), 0640)
	if err != nil {
		return err
	}
	return nil
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment(_ context.Context) error {
	_, err := os.Stat(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = replaceConfigDirectory(d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}
	return nil
}

// RemoveExperiment removes the experiment from the directories.
func (d *Directories) RemoveExperiment(_ context.Context) error {
	err := os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = symlink.Set(d.ExperimentPath, d.StablePath)
	if err != nil {
		return err
	}
	return nil
}

// copyDirectory copies a directory from source to target.
// It preserves the directory structure, file permissions, ownership and
// symlinks, and skips the config-backups directory: the backups must never be
// nested inside a copy of themselves, and promote must not replace the stable
// history wholesale.
func copyDirectory(ctx context.Context, sourcePath, targetPath string) error {
	return filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == backupDirName {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(targetPath, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyRegularFile(path, target, info)
	})
}

// copyRegularFile copies one regular file, preserving mode and ownership like
// cp -a does when run as root.
func copyRegularFile(src, dst string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := os.Lchown(dst, int(st.Uid), int(st.Gid)); err != nil {
			return err
		}
	}
	return nil
}

func isSameFile(file1, file2 string) bool {
	stat1, err := os.Stat(file1)
	if err != nil {
		return false
	}
	stat2, err := os.Stat(file2)
	if err != nil {
		return false
	}
	return os.SameFile(stat1, stat2)
}

// replaceConfigDirectory replaces the contents of two directories.
func replaceConfigDirectory(oldDir, newDir string) (err error) {
	backupDir, err := os.MkdirTemp(filepath.Dir(oldDir), "datadog-backup")
	if err != nil {
		return fmt.Errorf("could not create backup directory: %w", err)
	}
	defer os.RemoveAll(backupDir)
	backupPath := filepath.Join(backupDir, filepath.Base(oldDir))
	err = os.Rename(oldDir, backupPath)
	if err != nil {
		return fmt.Errorf("could not rename old directory: %w", err)
	}
	defer func() {
		if err != nil {
			rollbackErr := os.Rename(backupPath, oldDir)
			if rollbackErr != nil {
				err = fmt.Errorf("%w, rollback error: %w", err, rollbackErr)
			}
		}
	}()
	err = os.Rename(newDir, oldDir)
	if err != nil {
		return fmt.Errorf("could not rename new directory: %w", err)
	}
	return nil
}

// setFileOwnershipAndPermissions sets the ownership and permissions for a file based on its configFileSpec.
// If the user doesn't exist (e.g., in tests) or if we don't have permission
// to change ownership, the function logs a warning and continues without failing.
func setFileOwnershipAndPermissions(ctx context.Context, root *os.Root, path string, spec *configFileSpec) error {
	// Set file permissions
	if spec.mode != 0 {
		if err := root.Chmod(path, spec.mode); err != nil {
			return fmt.Errorf("error setting file permissions for %s: %w", path, err)
		}
	}

	// Set file ownership
	err := file.Chown(ctx, filepath.Join(root.Name(), path), spec.owner, spec.group)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
	}

	return nil
}
