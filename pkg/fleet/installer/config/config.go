// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains the logic to manage the config of the packages.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

var (
	// secRegex matches SEC[...] placeholders in config patches
	secRegex = regexp.MustCompile(`SEC\[.*?\]`)
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

// ReplaceSecrets replaces SEC[key] placeholders with decrypted values in the operations.
func ReplaceSecrets(operations *Operations, decryptedSecrets map[string]string) error {
	for key, decryptedValue := range decryptedSecrets {
		// Build the full key: SEC[key]
		fullKey := fmt.Sprintf("SEC[%s]", key)

		// Replace in all file operations
		for i := range operations.FileOperations {
			if bytes.Contains(operations.FileOperations[i].Patch, []byte(fullKey)) {
				operations.FileOperations[i].Patch = bytes.ReplaceAll(
					operations.FileOperations[i].Patch,
					[]byte(fullKey),
					[]byte(decryptedValue),
				)
			}
		}
	}

	// Verify all secrets have been replaced
	for _, operation := range operations.FileOperations {
		if secRegex.Match(operation.Patch) {
			return errors.New("secrets are not fully replaced, SEC[...] found in the config")
		}
	}

	return nil
}

// Apply applies the operations to the root.
func (o *Operations) Apply(ctx context.Context, rootPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, operation := range o.FileOperations {
		// TODO (go.1.25): we won't need rootPath in 1.25
		err := operation.apply(ctx, root, rootPath)
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

func (a *FileOperation) apply(ctx context.Context, root *os.Root, rootPath string) error {
	spec := getConfigFileSpec(a.FilePath)
	if spec == nil {
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
		previousJSONBytes, err := json.Marshal(convertYAML2UnmarshalToJSONMarshallable(previous))
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
		// Set proper ownership and permissions for the file
		fullPath := filepath.Join(rootPath, path)
		if err := setFileOwnershipAndPermissions(ctx, fullPath, spec); err != nil {
			return err
		}
		return nil
	case FileOperationCopy:
		destSpec := getConfigFileSpec(a.DestinationPath)
		if destSpec == nil {
			return fmt.Errorf("modifying config file %s is not allowed", a.DestinationPath)
		}

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

		err = root.WriteFile(destinationPath, srcContent, 0640)
		if err != nil {
			return err
		}

		// Set proper ownership and permissions for the destination file
		fullDestPath := filepath.Join(rootPath, destinationPath)
		if err := setFileOwnershipAndPermissions(ctx, fullDestPath, destSpec); err != nil {
			return err
		}
		return nil
	case FileOperationMove:
		destSpec := getConfigFileSpec(a.DestinationPath)
		if destSpec == nil {
			return fmt.Errorf("modifying config file %s is not allowed", a.DestinationPath)
		}

		err := ensureDir(root, destinationPath)
		if err != nil {
			return err
		}

		err = root.Rename(path, destinationPath)
		if err != nil {
			return err
		}

		// Set proper ownership and permissions for the destination file
		fullDestPath := filepath.Join(rootPath, destinationPath)
		if err := setFileOwnershipAndPermissions(ctx, fullDestPath, destSpec); err != nil {
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
		err := root.RemoveAll(path)
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
	return root.MkdirAll(dir, 0755)
}

// configFileSpec specifies a config file pattern, its ownership, and permissions.
type configFileSpec struct {
	pattern string
	owner   string
	group   string
	mode    os.FileMode
}

var (
	allowedConfigFiles = []configFileSpec{
		{pattern: "/datadog.yaml", owner: "dd-agent", group: "dd-agent", mode: 0640},
		{pattern: "/otel-config.yaml", owner: "dd-agent", group: "dd-agent", mode: 0640},
		{pattern: "/security-agent.yaml", owner: "root", group: "dd-agent", mode: 0640},
		{pattern: "/system-probe.yaml", owner: "root", group: "dd-agent", mode: 0640},
		{pattern: "/application_monitoring.yaml", owner: "root", group: "root", mode: 0644},
		{pattern: "/conf.d/*.yaml", owner: "dd-agent", group: "dd-agent", mode: 0640},
		{pattern: "/conf.d/*.d/*.yaml", owner: "dd-agent", group: "dd-agent", mode: 0640},
	}

	legacyPathPrefix = filepath.Join("managed", "datadog-agent", "stable")
)

func getConfigFileSpec(file string) *configFileSpec {
	normalizedFile := filepath.ToSlash(file)

	// Fallback for legacy files under the /managed directory
	if strings.HasPrefix(normalizedFile, "/managed") {
		filename := filepath.Base(normalizedFile)

		for _, spec := range allowedConfigFiles {
			// Skip patterns with nested paths (e.g., /conf.d/*.yaml)
			if strings.Count(spec.pattern, "/") > 1 {
				continue
			}

			// Extract just the filename from the pattern
			patternFilename := filepath.Base(spec.pattern)
			match, err := filepath.Match(patternFilename, filename)
			if err != nil {
				continue
			}
			if match {
				// Return a copy with the original pattern set to the full managed path
				return &configFileSpec{
					pattern: normalizedFile,
					owner:   spec.owner,
					group:   spec.group,
					mode:    spec.mode,
				}
			}
		}
		return &configFileSpec{pattern: normalizedFile, owner: "dd-agent", group: "dd-agent", mode: 0640}
	}

	for _, spec := range allowedConfigFiles {
		match, err := filepath.Match(spec.pattern, normalizedFile)
		if err != nil {
			continue
		}
		if match {
			return &spec
		}
	}
	return nil
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
	stableDatadogJSONBytes, err := json.Marshal(convertYAML2UnmarshalToJSONMarshallable(stableDatadogJSON))
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

// convertYAML2UnmarshalToJSONMarshallable converts a YAML unmarshalable to a JSON marshallable:
// yaml.v2 unmarshals nested maps to map[any]any, but json.Marshal expects map[string]any and
// fails for map[any]any. This function converts the map[any]any to map[string]any.
func convertYAML2UnmarshalToJSONMarshallable(i any) any {
	switch x := i.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, v := range x {
			if strKey, ok := k.(string); ok {
				m[strKey] = convertYAML2UnmarshalToJSONMarshallable(v)
			}
			// Skip non-string keys as they cannot be represented in JSON
		}
		return m
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			m[k] = convertYAML2UnmarshalToJSONMarshallable(v)
		}
		return m
	case []any:
		m := make([]any, len(x))
		for i, v := range x {
			m[i] = convertYAML2UnmarshalToJSONMarshallable(v)
		}
		return m
	}
	return i
}
