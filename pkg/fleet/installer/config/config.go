// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains the logic to manage the config of the packages.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	patch "gopkg.in/evanphx/json-patch.v4"
	"gopkg.in/yaml.v2"
)

// FileOperationType is the type of operation to perform on the config.
type FileOperationType string

const (
	// FileOperationPatch patches the config at the given path with the given JSON patch (RFC 6902).
	FileOperationPatch FileOperationType = "patch"
	// FileOperationMergePatch merges the config at the given path with the given JSON merge patch (RFC 7396).
	FileOperationMergePatch FileOperationType = "merge-patch"
	// FileOperationDelete deletes the config at the given path.
	FileOperationDelete FileOperationType = "delete"
	// FileOperationDeleteAll deletes the config at the given path and all its subdirectories.
	FileOperationDeleteAll FileOperationType = "delete-all"
	// FileOperationCopy copies the config at the given path to the given path.
	FileOperationCopy FileOperationType = "copy"
	// FileOperationMove moves the config at the given path to the given path.
	FileOperationMove FileOperationType = "move"
)

// Directories is the directories of the config.
type Directories struct {
	StablePath     string
	ExperimentPath string
}

// State is the state of the directories.
type State struct {
	StableDeploymentID     string
	ExperimentDeploymentID string
}

// Operations is the list of operations to perform on the configs.
type Operations struct {
	DeploymentID   string          `json:"deployment_id"`
	FileOperations []FileOperation `json:"file_operations"`
}

// Apply applies the operations to the root.
func (o *Operations) Apply(rootPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, operation := range o.FileOperations {
		// TODO (go.1.25): we won't need rootPath in 1.25
		err := operation.apply(root, rootPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// FileOperation is the operation to perform on a config.
type FileOperation struct {
	FileOperationType FileOperationType `json:"file_op"`
	FilePath          string            `json:"file_path"`
	DestinationPath   string            `json:"destination_path,omitempty"`
	Patch             json.RawMessage   `json:"patch,omitempty"`
}

func (a *FileOperation) apply(root *os.Root, rootPath string) error {
	if !configNameAllowed(a.FilePath) {
		return fmt.Errorf("modifying config file %s is not allowed", a.FilePath)
	}
	path := strings.TrimPrefix(a.FilePath, "/")
	destinationPath := strings.TrimPrefix(a.DestinationPath, "/")

	switch a.FileOperationType {
	case FileOperationPatch, FileOperationMergePatch:
		err := ensureDir(root, path)
		if err != nil {
			return err
		}
		file, err := root.OpenFile(path, os.O_RDWR|os.O_CREATE, 0640)
		if err != nil {
			return err
		}
		defer file.Close()
		previousYAMLBytes, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		previous := make(map[string]any)
		err = yaml.Unmarshal(previousYAMLBytes, &previous)
		if err != nil {
			return err
		}
		previousJSONBytes, err := json.Marshal(previous)
		if err != nil {
			return err
		}
		var newJSONBytes []byte
		switch a.FileOperationType {
		case FileOperationPatch:
			patch, err := patch.DecodePatch(a.Patch)
			if err != nil {
				return err
			}
			newJSONBytes, err = patch.Apply(previousJSONBytes)
			if err != nil {
				return err
			}
		case FileOperationMergePatch:
			newJSONBytes, err = patch.MergePatch(previousJSONBytes, a.Patch)
			if err != nil {
				return err
			}
		}
		var current map[string]any
		err = yaml.Unmarshal(newJSONBytes, &current)
		if err != nil {
			return err
		}
		currentYAMLBytes, err := yaml.Marshal(current)
		if err != nil {
			return err
		}
		err = file.Truncate(0)
		if err != nil {
			return err
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		_, err = file.Write(currentYAMLBytes)
		if err != nil {
			return err
		}
		return err
	case FileOperationCopy:
		// TODO(go.1.25): os.Root.MkdirAll and os.Root.WriteFile are only available starting go 1.25
		err := ensureDir(root, destinationPath)
		if err != nil {
			return err
		}

		srcFile, err := root.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		srcContent, err := io.ReadAll(srcFile)
		if err != nil {
			return err
		}

		// Create the destination with os.Root to ensure the path is clean
		destFile, err := root.Create(destinationPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = destFile.Write(srcContent)
		if err != nil {
			return err
		}
		return nil
	case FileOperationMove:
		// TODO(go.1.25): os.Root.Rename is only available starting go 1.25 so we'll use it instead
		err := ensureDir(root, destinationPath)
		if err != nil {
			return err
		}

		srcFile, err := root.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		srcContent, err := io.ReadAll(srcFile)
		if err != nil {
			return err
		}

		// Create the destination with os.Root to ensure the path is clean
		destFile, err := root.Create(destinationPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = destFile.Write(srcContent)
		if err != nil {
			return err
		}

		err = root.Remove(path)
		if err != nil {
			return err
		}
		return nil
	case FileOperationDelete:
		err := root.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case FileOperationDeleteAll:
		// TODO(go.1.25): os.Root.RemoveAll is only available starting go 1.25 so we'll use it instead
		// We can't get the path from os.Root, so we have to use the rootPath.
		err := os.RemoveAll(filepath.Join(rootPath, path))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", a.FileOperationType)
	}
}

func ensureDir(root *os.Root, filePath string) error {
	// Normalize path to forward slashes and remove leading slash
	normalizedPath := filepath.ToSlash(strings.TrimPrefix(filePath, "/"))

	// Get the directory part
	dir := path.Dir(normalizedPath)
	if dir == "." {
		return nil
	}
	currentRoot := root
	for part := range strings.SplitSeq(dir, "/") {
		if part == "" {
			continue
		}

		// Try to create the directory
		err := currentRoot.Mkdir(part, 0755)
		if err != nil && !os.IsExist(err) {
			return err
		}

		// Open the directory for the next iteration
		nextRoot, err := currentRoot.OpenRoot(part)
		if err != nil {
			return err
		}

		// Close the previous root if it's not the original root
		if currentRoot != root {
			currentRoot.Close()
		}
		currentRoot = nextRoot
	}

	// Close the final root if it's not the original root
	if currentRoot != root {
		currentRoot.Close()
	}
	return nil
}

var (
	allowedConfigFiles = []string{
		"/datadog.yaml",
		"/otel-config.yaml",
		"/security-agent.yaml",
		"/system-probe.yaml",
		"/application_monitoring.yaml",
		"/conf.d/*.yaml",
		"/conf.d/*.d/*.yaml",
	}

	legacyPathPrefix = filepath.Join("managed", "datadog-agent", "stable")
)

func configNameAllowed(file string) bool {
	// Normalize path to use forward slashes for consistent matching on all platforms
	normalizedFile := filepath.ToSlash(file)

	// Matching everything under the legacy /managed directory
	if strings.HasPrefix(normalizedFile, "/managed") {
		return true
	}

	for _, allowedFile := range allowedConfigFiles {
		match, err := filepath.Match(allowedFile, normalizedFile)
		if err != nil {
			return false
		}
		if match {
			return true
		}
	}
	return false
}

func buildOperationsFromLegacyInstaller(rootPath string) []FileOperation {
	var allOps []FileOperation

	// /etc/datadog-agent/
	realRootPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return allOps
	}

	// Check if stable is a symlink or not. If it's not we can return early
	// because the migration is already done
	existingStablePath := filepath.Join(rootPath, legacyPathPrefix)
	info, err := os.Lstat(existingStablePath)
	if err != nil {
		if os.IsNotExist(err) {
			return allOps
		}
		return allOps
	}
	// If it's not a symlink, we can return early
	if info.Mode()&os.ModeSymlink == 0 {
		return allOps
	}

	// Eval legacyPathPrefix symlink from rootPath
	// /etc/datadog-agent/managed/datadog-agent/aaaa-bbbb-cccc
	stableDirPath, err := filepath.EvalSymlinks(filepath.Join(realRootPath, legacyPathPrefix))
	if err != nil {
		return allOps
	}

	// managed/datadog-agent/aaaa-bbbb-cccc
	managedDirSubPath, err := filepath.Rel(realRootPath, stableDirPath)
	if err != nil {
		return allOps
	}

	// Recursively delete targetPath/
	// RemoveAll removes symlinks but not the content they point to as it uses os.Remove first
	allOps = append(allOps, FileOperation{
		FileOperationType: FileOperationDeleteAll,
		FilePath:          "/managed",
	})

	err = filepath.WalkDir(stableDirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		op, err := buildOperationsFromLegacyConfigFile(path, realRootPath, managedDirSubPath)
		if err != nil {
			return err
		}

		allOps = append(allOps, op)
		return nil
	})
	if err != nil {
		return []FileOperation{}
	}

	return allOps
}

func buildOperationsFromLegacyConfigFile(fullFilePath, fullRootPath, managedDirSubPath string) (FileOperation, error) {
	// Read the stable config file
	stableDatadogYAML, err := os.ReadFile(fullFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return FileOperation{}, nil
		}
		return FileOperation{}, err
	}

	var stableDatadogJSON map[string]any
	err = yaml.Unmarshal(stableDatadogYAML, &stableDatadogJSON)
	if err != nil {
		return FileOperation{}, fmt.Errorf("failed to unmarshal stable datadog.yaml: %w", err)
	}
	stableDatadogJSONBytes, err := json.Marshal(stableDatadogJSON)
	if err != nil {
		return FileOperation{}, fmt.Errorf("failed to marshal stable datadog.yaml: %w", err)
	}

	managedFilePath, err := filepath.Rel(fullRootPath, fullFilePath)
	if err != nil {
		return FileOperation{}, err
	}
	fPath, err := filepath.Rel(managedDirSubPath, managedFilePath)
	if err != nil {
		return FileOperation{}, err
	}

	op := FileOperation{
		FileOperationType: FileOperationType(FileOperationMergePatch),
		FilePath:          "/" + strings.TrimPrefix(fPath, "/"),
		Patch:             stableDatadogJSONBytes,
	}
	if fPath == "application_monitoring.yaml" {
		// Copy in managed directory
		op = FileOperation{
			FileOperationType: FileOperationMergePatch,
			FilePath:          "/" + filepath.Join("managed", "datadog-agent", "stable", fPath),
			Patch:             stableDatadogJSONBytes,
		}
	}

	return op, nil
}
