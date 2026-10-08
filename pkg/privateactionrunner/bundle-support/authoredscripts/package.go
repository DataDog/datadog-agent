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
	Manifest              *Manifest
	Command               []string
	ExecutableDirectories []string
}

func LoadPackage(fqn string, descriptor Descriptor, artifact LocalArtifact) (*Package, error) {
	manifest, err := loadManifest(artifact.ScriptDirectory())
	if err != nil {
		return nil, err
	}
	if err := validatePackageIdentity(fqn, descriptor, manifest); err != nil {
		return nil, err
	}

	manifestCommand := manifest.Command.Entrypoint
	commandPath, err := resolvePackageFile(artifact.ScriptDirectory(), manifestCommand)
	if err != nil {
		return nil, fmt.Errorf("invalid authored-script command: %w", err)
	}
	command := append([]string{commandPath}, manifest.Command.Args...)

	executableDirectories := make([]string, 0, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		binDir := dependency.BinDir
		if binDir == "" {
			binDir = "."
		}
		directory, err := resolvePackageDirectory(artifact.DependencyDirectory(dependency.Name), binDir)
		if err != nil {
			return nil, fmt.Errorf("invalid authored-script dependency %q: %w", dependency.Name, err)
		}
		executableDirectories = append(executableDirectories, directory)
	}

	return &Package{
		Manifest:              manifest,
		Command:               command,
		ExecutableDirectories: executableDirectories,
	}, nil
}

func validatePackageIdentity(fqn string, descriptor Descriptor, manifest *Manifest) error {
	if !strings.EqualFold(descriptor.FQN, fqn) {
		return fmt.Errorf("authored-script descriptor FQN %q does not match catalog key %q", descriptor.FQN, fqn)
	}
	if !strings.EqualFold(manifest.FQN, descriptor.FQN) {
		return fmt.Errorf("authored-script manifest FQN %q does not match descriptor FQN %q", manifest.FQN, descriptor.FQN)
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

func resolvePackageDirectory(root, path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("path %q is not relative to the package", path)
	}

	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", fmt.Errorf("could not open package root: %w", err)
	}
	defer rootHandle.Close()

	info, err := rootHandle.Stat(path)
	if err != nil {
		return "", fmt.Errorf("could not access directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", path)
	}

	return filepath.Join(root, path), nil
}
