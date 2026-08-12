// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
)

const toolsDirectory = "tools"

// Package contains a validated authored script and the paths needed to execute it.
type Package struct {
	Manifest        *Manifest
	Directory       string
	Command         []string
	ToolDirectories []string
}

func LoadPackage(fqn string, descriptor artifacts.Descriptor, artifact artifacts.LocalArtifact) (*Package, error) {
	if err := validateArtifactDirectory(artifact.Directory); err != nil {
		return nil, err
	}

	manifest, err := loadManifest(artifact.Directory)
	if err != nil {
		return nil, err
	}
	if err := validatePackageIdentity(fqn, descriptor, manifest); err != nil {
		return nil, err
	}

	commandPath, err := resolvePackageFile(artifact.Directory, filepath.Join(scriptDirectory, manifest.Config.Command[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid authored-script command: %w", err)
	}
	command := append([]string{commandPath}, manifest.Config.Command[1:]...)

	toolDirectories := make([]string, 0, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		toolPath, err := resolvePackageFile(artifact.Directory, filepath.Join(toolsDirectory, dependency.Name, dependency.Name))
		if err != nil {
			return nil, fmt.Errorf("invalid authored-script dependency %q: %w", dependency.Name, err)
		}
		toolDirectories = append(toolDirectories, filepath.Dir(toolPath))
	}

	return &Package{
		Manifest:        manifest,
		Directory:       artifact.Directory,
		Command:         command,
		ToolDirectories: toolDirectories,
	}, nil
}

func validateArtifactDirectory(directory string) error {
	if directory == "" {
		return errors.New("authored-script artifact directory is required")
	}
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("authored-script artifact directory %q is not absolute", directory)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("could not access authored-script artifact directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("authored-script artifact path %q is not a directory", directory)
	}
	return nil
}

func validatePackageIdentity(fqn string, descriptor artifacts.Descriptor, manifest *Manifest) error {
	if manifest.FQN != fqn {
		return fmt.Errorf("authored-script manifest FQN %q does not match catalog key %q", manifest.FQN, fqn)
	}
	if manifest.Package != descriptor.Name {
		return fmt.Errorf("authored-script manifest package %q does not match artifact %q", manifest.Package, descriptor.Name)
	}
	if manifest.Version != descriptor.Version {
		return fmt.Errorf("authored-script manifest version %q does not match artifact version %q", manifest.Version, descriptor.Version)
	}
	return nil
}

func resolvePackageFile(root, path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("path %q is not relative to the package", path)
	}

	resolvedPath, err := securejoin.SecureJoin(root, path)
	if err != nil {
		return "", fmt.Errorf("could not resolve path %q: %w", path, err)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("could not access file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path %q is not a regular file", path)
	}
	return resolvedPath, nil
}
