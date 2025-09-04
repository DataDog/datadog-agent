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
	"gopkg.in/yaml.v3"
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
	stableDeploymentID, err := os.ReadFile(filepath.Join(d.StablePath, deploymentIDFile))
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	experimentDeploymentID, err := os.ReadFile(filepath.Join(d.ExperimentPath, deploymentIDFile))
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	return State{
		StableDeploymentID:     string(stableDeploymentID),
		ExperimentDeploymentID: string(experimentDeploymentID),
	}, nil
}

// WriteExperiment writes the experiment to the directories.
func (d *Directories) WriteExperiment(operations Operations) error {
	err := os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = os.MkdirAll(d.ExperimentPath, 0755)
	if err != nil {
		return err
	}
	err = copyDirectory(d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}
	err = operations.Apply(d.ExperimentPath)
	if err != nil {
		return err
	}
	return nil
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment() error {
	// check if experiment path exists using os
	_, err := os.Stat(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = swapConfigDirectories(d.ExperimentPath, d.StablePath)
	if err != nil {
		return err
	}
	return nil
}

// RemoveExperiment removes the experiment from the directories.
func (d *Directories) RemoveExperiment() error {
	err := os.RemoveAll(d.ExperimentPath)
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
	Patch             json.RawMessage   `json:"patch,omitempty"`
}

func (a *FileOperation) apply(root *os.Root) error {
	if !configNameAllowed(a.FilePath) {
		return fmt.Errorf("modifying config file %s is not allowed", a.FilePath)
	}
	path := strings.TrimPrefix(a.FilePath, "/")

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
