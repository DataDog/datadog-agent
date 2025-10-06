// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains the logic to manage the config of the packages.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	patch "gopkg.in/evanphx/json-patch.v4"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
)

const (
	deploymentIDFile = ".deployment-id"
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
	if runtime.GOOS == "windows" {
		// On windows, experiments are not supported yet for configuration.
		return operations.Apply(d.StablePath)
	}
	err := os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = copyDirectory(ctx, d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}

	operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)

	err = operations.Apply(d.ExperimentPath)
	if err != nil {
		return err
	}
	return nil
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment(_ context.Context) error {
	if runtime.GOOS == "windows" {
		// On windows, experiments are not supported yet for configuration.
		return nil
	}
	// check if experiment path exists using os
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
	if runtime.GOOS == "windows" {
		// On windows, experiments are not supported yet for configuration.
		return nil
	}
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
	for _, operation := range o.FileOperations {
		err := operation.apply(root)
		if err != nil {
			return err
		}
	}
	err = os.WriteFile(filepath.Join(rootPath, deploymentIDFile), []byte(o.DeploymentID), 0644)
	if err != nil {
		return err
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

func (a *FileOperation) apply(root *os.Root) error {
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
		file, err := root.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
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
		err := ensureDir(root, destinationPath)
		if err != nil {
			return err
		}

		srcContent, err := root.ReadFile(path)
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
		err := ensureDir(root, destinationPath)
		if err != nil {
			return err
		}

		srcContent, err := root.ReadFile(path)
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
	default:
		return fmt.Errorf("unknown operation type: %s", a.FileOperationType)
	}
}

func ensureDir(root *os.Root, filePath string) error {
	dir := path.Dir(filePath)
	if dir == "." {
		return nil
	}
	for part := range strings.SplitSeq(dir, "/") {
		if part == "" {
			continue
		}
		err := root.Mkdir(part, 0755)
		if err != nil && !os.IsExist(err) {
			return err
		}
		root, err = root.OpenRoot(part)
		if err != nil {
			return err
		}
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
	// Matching everything under the legacy /managed directory
	if strings.HasPrefix(file, "/managed") {
		return true
	}

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

func buildOperationsFromLegacyInstaller(rootPath string) []FileOperation {
	var allOps []FileOperation

	// /etc/datadog-agent/
	realRootPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
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

	err = filepath.Walk(stableDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Ignore application_monitoring.yaml as we need to keep it in the managed directory
		if strings.HasSuffix(path, "application_monitoring.yaml") {
			return nil
		}

		ops, err := buildOperationsFromLegacyConfigFile(path, realRootPath, managedDirSubPath)
		if err != nil {
			return err
		}

		allOps = append(allOps, ops...)
		return nil
	})
	if err != nil {
		return []FileOperation{}
	}

	return allOps
}

func buildOperationsFromLegacyConfigFile(fullFilePath, fullRootPath, managedDirSubPath string) ([]FileOperation, error) {
	var ops []FileOperation

	// Read the stable config file
	stableDatadogYAML, err := os.ReadFile(fullFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ops, nil
		}
		return ops, err
	}

	// Since the config is YAML, we need to convert it to JSON
	// 1. Parse the YAML in interface{}
	// 2. Serialize the interface{} to JSON
	var stableDatadogJSON interface{}
	err = yaml.Unmarshal(stableDatadogYAML, &stableDatadogJSON)
	if err != nil {
		return ops, err
	}
	stableDatadogJSONBytes, err := json.Marshal(stableDatadogJSON)
	if err != nil {
		return ops, err
	}

	managedFilePath, err := filepath.Rel(fullRootPath, fullFilePath)
	if err != nil {
		return ops, err
	}
	fPath, err := filepath.Rel(managedDirSubPath, managedFilePath)
	if err != nil {
		return ops, err
	}

	// Add the merge patch operation
	ops = append(ops, FileOperation{
		FileOperationType: FileOperationType(FileOperationMergePatch),
		FilePath:          "/" + strings.TrimPrefix(fPath, "/"),
		Patch:             stableDatadogJSONBytes,
	})

	// Add the delete operation for the old file
	ops = append(ops, FileOperation{
		FileOperationType: FileOperationType(FileOperationDelete),
		FilePath:          "/" + strings.TrimPrefix(managedFilePath, "/"),
	})

	return ops, nil
}
