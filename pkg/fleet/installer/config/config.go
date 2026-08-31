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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	patch "github.com/evanphx/json-patch/v5"
	"github.com/itchyny/gojq"
	"go.yaml.in/yaml/v2"
)

// FileOperationType is the type of operation to perform on the config.
type FileOperationType string

const (
	// FileOperationPatch patches the config at the given path with the given JSON patch (RFC 6902).
	FileOperationPatch FileOperationType = "patch"
	// FileOperationMergePatch merges the config at the given path with the given JSON merge patch (RFC 7396).
	FileOperationMergePatch FileOperationType = "merge-patch"
	// FileOperationJQ transforms the config at the given path by running an arbitrary jq transform over it.
	FileOperationJQ FileOperationType = "jq"
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

		// Replace in all file operations. Patch and jq Arguments are both raw JSON blobs,
		// so secrets are substituted with a flat textual replacement that is agnostic to
		// nesting depth. For jq, secrets live in the arguments rather than the transform
		// text, so the transform stays a static program and secret values are never spliced
		// into the jq source.
		for i := range operations.FileOperations {
			if bytes.Contains(operations.FileOperations[i].Patch, []byte(fullKey)) {
				operations.FileOperations[i].Patch = bytes.ReplaceAll(
					operations.FileOperations[i].Patch,
					[]byte(fullKey),
					[]byte(decryptedValue),
				)
			}
			if bytes.Contains(operations.FileOperations[i].Arguments, []byte(fullKey)) {
				operations.FileOperations[i].Arguments = bytes.ReplaceAll(
					operations.FileOperations[i].Arguments,
					[]byte(fullKey),
					[]byte(decryptedValue),
				)
			}
		}
	}

	// Verify all secrets have been replaced. The transform text is also checked so that a
	// stray SEC[...] embedded directly in a transform (which is unsupported) fails loudly
	// rather than reaching gojq unsubstituted.
	for _, operation := range operations.FileOperations {
		if secRegex.Match(operation.Patch) || secRegex.MatchString(operation.Transform) || secRegex.Match(operation.Arguments) {
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
		err := operation.apply(ctx, root)
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
	Transform         string            `json:"transform,omitempty"`
	Arguments         json.RawMessage   `json:"arguments,omitempty"`
}

func (a *FileOperation) apply(ctx context.Context, root *os.Root) error {
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
			return fmt.Errorf("could not parse config file %q as YAML (fix the syntax error reported below and retry): %w", a.FilePath, err)
		}
		previousJSONBytes, err := json.Marshal(convertYAML2UnmarshalToJSONMarshallable(previous))
		if err != nil {
			return fmt.Errorf("could not serialize config file %q: %w", a.FilePath, err)
		}
		var newJSONBytes []byte
		switch a.FileOperationType {
		case FileOperationPatch:
			patch, err := patch.DecodePatch(a.Patch)
			if err != nil {
				return fmt.Errorf("could not decode patch for config file %q: %w", a.FilePath, err)
			}
			newJSONBytes, err = patch.Apply(previousJSONBytes)
			if err != nil {
				return fmt.Errorf("could not apply patch to config file %q: %w", a.FilePath, err)
			}
		case FileOperationMergePatch:
			newJSONBytes, err = patch.MergePatch(previousJSONBytes, a.Patch)
			if err != nil {
				return fmt.Errorf("could not apply merge patch to config file %q: %w", a.FilePath, err)
			}
		}
		var current map[string]any
		err = yaml.Unmarshal(newJSONBytes, &current)
		if err != nil {
			return fmt.Errorf("could not parse patched config for file %q: %w", a.FilePath, err)
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
		if err := setFileOwnershipAndPermissions(ctx, root, path, spec); err != nil {
			return err
		}
		return nil
	case FileOperationJQ:
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
		parsed, err := gojq.Parse(a.Transform)
		if err != nil {
			return fmt.Errorf("failed to parse jq transform: %w", err)
		}
		// Arguments are exposed to the transform as named jq variables ($name), keeping
		// the transform a static program with its values supplied separately. gojq matches
		// variable names to values by position, so iterate the sorted keys for both.
		arguments := make(map[string]any)
		if len(a.Arguments) > 0 {
			if err := json.Unmarshal(a.Arguments, &arguments); err != nil {
				return fmt.Errorf("failed to parse jq arguments: %w", err)
			}
		}
		argNames := make([]string, 0, len(arguments))
		for name := range arguments {
			argNames = append(argNames, name)
		}
		sort.Strings(argNames)
		variables := make([]string, len(argNames))
		argValues := make([]any, len(argNames))
		for i, name := range argNames {
			variables[i] = "$" + name
			argValues[i] = arguments[name]
		}
		code, err := gojq.Compile(parsed, gojq.WithVariables(variables))
		if err != nil {
			return fmt.Errorf("failed to compile jq transform: %w", err)
		}
		// Run the transform over the normalized config. It may yield any number of
		// outputs; each one is written as its own YAML document so arbitrary
		// transformations (including those producing multiple documents) are supported.
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		iter := code.RunWithContext(ctx, convertYAML2UnmarshalToJSONMarshallable(previous), argValues...)
		outputs := 0
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				return fmt.Errorf("failed to run jq transform: %w", err)
			}
			if err := encoder.Encode(v); err != nil {
				return err
			}
			outputs++
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		if outputs == 0 {
			return fmt.Errorf("jq transform %q produced no output", a.Transform)
		}
		err = file.Truncate(0)
		if err != nil {
			return err
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		_, err = file.Write(buf.Bytes())
		if err != nil {
			return err
		}
		// Set proper ownership and permissions for the file
		if err := setFileOwnershipAndPermissions(ctx, root, path, spec); err != nil {
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
		if err := setFileOwnershipAndPermissions(ctx, root, destinationPath, destSpec); err != nil {
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
		if err := setFileOwnershipAndPermissions(ctx, root, destinationPath, destSpec); err != nil {
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

	legacyManagedLinkPaths = []string{
		"managed/datadog-agent/stable",
		"managed/datadog-agent/experiment",
	}
)

func getConfigFileSpec(file string) *configFileSpec {
	normalizedFile := filepath.ToSlash(file)

	// Fallback for files in the legacy managed config tree. The exact root is
	// allowed because legacy migration deletes it before recreating the config.
	legacyConfigPath := "/" + filepath.ToSlash(legacyPathPrefix)
	if normalizedFile == "/managed" ||
		normalizedFile == legacyConfigPath ||
		strings.HasPrefix(normalizedFile, legacyConfigPath+"/") {
		filename := path.Base(normalizedFile)

		for _, spec := range allowedConfigFiles {
			// Skip patterns with nested paths (e.g., /conf.d/*.yaml)
			if strings.Count(spec.pattern, "/") > 1 {
				continue
			}

			// Extract just the filename from the pattern
			patternFilename := path.Base(spec.pattern)
			match, err := path.Match(patternFilename, filename)
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
		match, err := path.Match(spec.pattern, normalizedFile)
		if err != nil {
			continue
		}
		if match {
			return &spec
		}
	}
	return nil
}

func isManagedConfigYAML(relativePath string) bool {
	return strings.EqualFold(filepath.Ext(relativePath), ".yaml") &&
		getConfigFileSpec("/"+filepath.ToSlash(relativePath)) != nil
}

func removeConfigFilesMissingFromSource(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	emptiedDirs := make(map[string]struct{})
	err = walkFiles(targetRoot.FS(), ".", func(relativePath string, _ fs.DirEntry) error {
		if !isManagedConfigYAML(relativePath) {
			return nil
		}
		removed, err := removeConfigFileMissingFromSource(sourceRoot, targetRoot, relativePath)
		if err != nil {
			return err
		}
		if removed {
			if dir := path.Dir(relativePath); dir != "." {
				emptiedDirs[dir] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	pruneEmptyManagedDirs(sourceRoot, targetRoot, emptiedDirs)
	return nil
}

// removeConfigFileMissingFromSource removes relativePath from the target when the source
// does not have it. It reports whether the file was removed.
func removeConfigFileMissingFromSource(sourceRoot, targetRoot *os.Root, relativePath string) (bool, error) {
	_, err := sourceRoot.Lstat(relativePath)
	if err == nil {
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("could not check source config file %q: %w", relativePath, err)
	}
	if err := targetRoot.Remove(relativePath); err != nil {
		return false, fmt.Errorf("could not remove config file %q during rollback: %w", relativePath, err)
	}
	return true, nil
}

// pruneEmptyManagedDirs removes the directories that the cleanup above just emptied.
//
// Removing a config file leaves its directory behind, so an experiment that added
// conf.d/<check>.d/conf.yaml would otherwise leave an empty conf.d/<check>.d in the stable
// directory after every rollback. Only directories the cleanup actually emptied are
// considered, and a directory is removed only when the source does not have it and it holds
// no remaining entries, so unmanaged files always keep their directory alive.
//
// Pruning is best effort: a directory that cannot be removed is left in place rather than
// failing an otherwise successful rollback.
func pruneEmptyManagedDirs(sourceRoot, targetRoot *os.Root, emptiedDirs map[string]struct{}) {
	for _, dir := range dirsDeepestFirst(emptiedDirs) {
		if _, err := sourceRoot.Lstat(dir); err == nil || !os.IsNotExist(err) {
			// The source still has this directory, or we cannot tell: keep it.
			continue
		}
		if !isEmptyDir(targetRoot, dir) {
			continue
		}
		// A directory that cannot be removed is left in place. Its parents cannot be
		// empty either, so the rest of the pass skips them on its own.
		_ = targetRoot.Remove(dir)
	}
}

// dirsDeepestFirst expands dirs with their parent directories and orders them so that a
// directory always comes before its parent, letting a parent that is emptied by the removal
// of its last child be pruned in the same pass.
func dirsDeepestFirst(dirs map[string]struct{}) []string {
	all := make(map[string]struct{}, len(dirs))
	for dir := range dirs {
		for d := dir; d != "." && d != "/" && d != ""; d = path.Dir(d) {
			all[d] = struct{}{}
		}
	}

	ordered := make([]string, 0, len(all))
	for d := range all {
		ordered = append(ordered, d)
	}
	// A child path is its parent plus more, so reverse lexical order puts every child
	// ahead of its parent.
	sort.Sort(sort.Reverse(sort.StringSlice(ordered)))
	return ordered
}

// isEmptyDir reports whether dir holds no entries at all, including unmanaged ones.
func isEmptyDir(root *os.Root, dir string) bool {
	f, err := root.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == io.EOF
}

func walkFiles(fsys fs.FS, root string, fn func(relativePath string, entry fs.DirEntry) error) error {
	return fs.WalkDir(fsys, root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath := filePath
		if root != "." {
			relativePath = strings.TrimPrefix(filePath, root+"/")
		}
		return fn(relativePath, entry)
	})
}

func reconcileLegacyManagedLinksBeforeCopy(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	for _, relativePath := range legacyManagedLinkPaths {
		sourceInfo, err := sourceRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not inspect source legacy link %q: %w", relativePath, err)
		}
		if sourceInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		if err := targetRoot.RemoveAll(relativePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not reconcile legacy link %q: %w", relativePath, err)
		}
	}
	return nil
}

func verifyLegacyManagedLinksCopied(sourceRoot, targetRoot *os.Root) error {
	for _, relativePath := range legacyManagedLinkPaths {
		sourceInfo, err := sourceRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not inspect source legacy link %q: %w", relativePath, err)
		}
		if sourceInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		sourceTarget, err := sourceRoot.Readlink(relativePath)
		if err != nil {
			return fmt.Errorf("could not read source legacy link %q: %w", relativePath, err)
		}

		targetInfo, err := targetRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			return fmt.Errorf("legacy link %q was not copied", relativePath)
		}
		if err != nil {
			return fmt.Errorf("could not inspect copied legacy link %q: %w", relativePath, err)
		}
		if targetInfo.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("legacy link %q was not restored as a symlink", relativePath)
		}

		targetTarget, err := targetRoot.Readlink(relativePath)
		if err != nil {
			return fmt.Errorf("could not read copied legacy link %q: %w", relativePath, err)
		}
		if targetTarget != sourceTarget {
			return fmt.Errorf("legacy link %q target mismatch: got %q, want %q", relativePath, targetTarget, sourceTarget)
		}
	}
	return nil
}

func verifyConfigFilesCopied(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	err = walkFiles(sourceRoot.FS(), ".", func(relativePath string, _ fs.DirEntry) error {
		if !strings.EqualFold(path.Ext(relativePath), ".yaml") {
			return nil
		}

		_, err := targetRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			return fmt.Errorf("config file %q was not copied", relativePath)
		}
		if err != nil {
			return fmt.Errorf("could not check copied config file %q: %w", relativePath, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return verifyLegacyManagedLinksCopied(sourceRoot, targetRoot)
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
		return FileOperation{}, fmt.Errorf("could not parse config file %q as YAML (fix the syntax error reported below and retry): %w", fullFilePath, err)
	}
	stableDatadogJSONBytes, err := json.Marshal(convertYAML2UnmarshalToJSONMarshallable(stableDatadogJSON))
	if err != nil {
		return FileOperation{}, fmt.Errorf("could not serialize config file %q: %w", fullFilePath, err)
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
	case time.Time:
		// yaml.v2 unmarshals timestamps to time.Time, which gojq cannot handle.
		// Use RFC3339Nano to preserve sub-second precision, matching json.Marshal.
		return x.Format(time.RFC3339Nano)
	}
	return i
}
