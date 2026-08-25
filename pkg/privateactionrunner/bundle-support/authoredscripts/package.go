// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Package contains a validated authored script and the paths needed to execute it.
type Package struct {
	Manifest  *Manifest
	Directory string
	Command   []string
	ToolPaths []string
}

func LoadPackage(fqn string, descriptor Descriptor, artifact LocalArtifact) (*Package, error) {
	manifest, err := loadManifest(artifact.Directory)
	if err != nil {
		return nil, err
	}
	if err := validatePackageIdentity(fqn, descriptor, manifest); err != nil {
		return nil, err
	}

	manifestCommand := manifest.Config.Command[0]
	if !filepath.IsLocal(manifestCommand) {
		return nil, fmt.Errorf("invalid authored-script command path %q: path must be local to the script directory", manifestCommand)
	}
	commandPath, err := resolvePackageFile(artifact.Directory, filepath.Join(scriptDirectory, manifestCommand))
	if err != nil {
		return nil, fmt.Errorf("invalid authored-script command: %w", err)
	}
	command := append([]string{commandPath}, manifest.Config.Command[1:]...)

	toolPaths := make([]string, 0, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		if !isDependencyName(dependency.Name) {
			return nil, fmt.Errorf("invalid authored-script dependency name %q: name must be a single path component", dependency.Name)
		}
		toolPath, err := resolvePackageFile(artifact.Directory, filepath.Join(scriptDirectory, dependency.Name))
		if err != nil {
			return nil, fmt.Errorf("invalid authored-script dependency %q: %w", dependency.Name, err)
		}
		toolPaths = append(toolPaths, toolPath)
	}

	return &Package{
		Manifest:  manifest,
		Directory: artifact.Directory,
		Command:   command,
		ToolPaths: toolPaths,
	}, nil
}

func isDependencyName(name string) bool {
	return name != "." && filepath.IsLocal(name) && !strings.ContainsAny(name, `/\\`)
}

func validatePackageIdentity(fqn string, descriptor Descriptor, manifest *Manifest) error {
	if descriptor.Package != fqn {
		return fmt.Errorf("authored-script descriptor package %q does not match catalog key %q", descriptor.Package, fqn)
	}
	if manifest.FQN != fqn {
		return fmt.Errorf("authored-script manifest FQN %q does not match catalog key %q", manifest.FQN, fqn)
	}
	if manifest.Version != descriptor.Version {
		return fmt.Errorf("authored-script manifest version %q does not match artifact version %q", manifest.Version, descriptor.Version)
	}
	return nil
}

func resolvePackageFile(root, path string) (string, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", fmt.Errorf("could not open package root: %w", err)
	}
	defer rootHandle.Close()

	info, err := rootHandle.Stat(path)
	if err != nil {
		return "", fmt.Errorf("could not access file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path %q is not a regular file", path)
	}

	return filepath.Join(root, path), nil
}
