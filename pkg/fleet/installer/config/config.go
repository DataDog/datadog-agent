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

// Operations is the list of operations to perform on the configs.
type Operations struct {
	DeploymentID   string          `json:"deployment_id"`
	FileOperations []FileOperation `json:"file_operations"`
}

// FileOperation is the operation to perform on a config.
type FileOperation struct {
	FileOperationType FileOperationType `json:"file_op"`
	FilePath          string            `json:"file_path"`
	Patch             json.RawMessage   `json:"patch,omitempty"`
}

// Apply applies the operation to the root.
func (a *FileOperation) Apply(root *os.Root) error {
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

// writeConfigSymlinks writes `.override` symlinks to help surface configurations to the user
func writeConfigSymlinks(userDir string, fleetDir string) error {
	userFiles, err := os.ReadDir(userDir)
	if err != nil {
		return fmt.Errorf("could not list user config files: %w", err)
	}
	for _, userFile := range userFiles {
		if userFile.Type()&os.ModeSymlink != 0 && strings.HasSuffix(userFile.Name(), ".override") {
			err = os.Remove(filepath.Join(userDir, userFile.Name()))
			if err != nil {
				return fmt.Errorf("could not remove existing symlink: %w", err)
			}
		}
	}
	var files []string
	fleetFiles, err := os.ReadDir(fleetDir)
	if err != nil {
		return fmt.Errorf("could not list fleet config files: %w", err)
	}
	for _, fleetFile := range fleetFiles {
		files = append(files, fleetFile.Name())
	}
	for _, file := range files {
		overrideFile := file + ".override"
		err = os.Symlink(filepath.Join(fleetDir, file), filepath.Join(userDir, overrideFile))
		if err != nil {
			return fmt.Errorf("could not create symlink: %w", err)
		}
	}
	return nil
}
