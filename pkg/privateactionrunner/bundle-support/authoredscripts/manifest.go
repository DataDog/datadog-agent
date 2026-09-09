// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const (
	manifestSchemaVersion    = "v1"
	scriptDirectory          = "script"
	manifestFile             = "package.yaml"
	maxManifestSize          = 1 << 20
	environmentKindValue     = "value"
	environmentKindFile      = "file"
	environmentKindDirectory = "directory"
)

// Manifest describes how an authored-script package is executed.
type Manifest struct {
	SchemaVersion string       `yaml:"schema-version"`
	Package       string       `yaml:"dd-package"`
	Version       string       `yaml:"version"`
	Title         string       `yaml:"title"`
	Description   string       `yaml:"description"`
	FQN           string       `yaml:"fqn"`
	URL           string       `yaml:"url"`
	Config        ScriptConfig `yaml:"config"`
	Dependencies  []Dependency `yaml:"dependencies"`
}

// ScriptConfig describes the command, inputs, and environment available to a script.
type ScriptConfig struct {
	Command           []string               `yaml:"command"`
	ParameterSchema   map[string]interface{} `yaml:"parameterSchema"`
	AllowedEnvVars    []string               `yaml:"allowedEnvVars"`
	SetSessionEnvVars []EnvironmentVariable  `yaml:"setSessionEnvVars"`
}

// EnvironmentVariable describes an environment value created for a script. File and
// directory values are paths relative to the session directory.
type EnvironmentVariable struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
	Kind  string `yaml:"kind"`
}

// Dependency identifies a tool bundled with an authored-script package.
type Dependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// loadManifest reads and validates the package.yaml manifest for an authored-script
// package that has already been downloaded and extracted to artifactDirectory.
func loadManifest(artifactDirectory string) (*Manifest, error) {
	file, err := openPackageFile(artifactDirectory, filepath.Join(scriptDirectory, manifestFile))
	if err != nil {
		return nil, fmt.Errorf("could not open authored-script manifest: %w", err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not read authored-script manifest: %w", err)
	}
	if len(contents) > maxManifestSize {
		return nil, fmt.Errorf("authored-script manifest exceeds the %d byte limit", maxManifestSize)
	}

	manifest := &Manifest{}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(manifest); err != nil {
		return nil, fmt.Errorf("could not decode authored-script manifest: %w", err)
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, errors.New("authored-script manifest must contain exactly one YAML document")
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

func validateManifest(manifest *Manifest) error {
	if manifest.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("unsupported authored-script manifest schema version %q", manifest.SchemaVersion)
	}
	if manifest.Package == "" {
		return errors.New("authored-script manifest package is required")
	}
	if manifest.Version == "" {
		return errors.New("authored-script manifest version is required")
	}
	if manifest.FQN == "" {
		return errors.New("authored-script manifest FQN is required")
	}
	if len(manifest.Config.Command) == 0 || manifest.Config.Command[0] == "" {
		return errors.New("authored-script manifest command is required")
	}
	if err := validateManifestEnvironmentVariables("session", manifest.Config.SetSessionEnvVars); err != nil {
		return err
	}
	for _, dependency := range manifest.Dependencies {
		if dependency.Name == "" || dependency.Version == "" {
			return errors.New("authored-script manifest dependencies require a name and version")
		}
	}
	return nil
}

func validateManifestEnvironmentVariables(scope string, variables []EnvironmentVariable) error {
	for _, environmentVariable := range variables {
		if environmentVariable.Name == "" || environmentVariable.Value == "" || environmentVariable.Kind == "" {
			return fmt.Errorf("authored-script manifest %s environment variables require a name, value, and kind", scope)
		}
		switch environmentVariable.Kind {
		case environmentKindValue, environmentKindFile, environmentKindDirectory:
		default:
			return fmt.Errorf("authored-script manifest %s environment variable %q has unsupported kind %q", scope, environmentVariable.Name, environmentVariable.Kind)
		}
	}
	return nil
}

func openPackageFile(root, path string) (*os.File, error) {
	if path == "" {
		return nil, errors.New("path is empty")
	}
	if !filepath.IsLocal(path) {
		return nil, fmt.Errorf("path %q is not relative to the package", path)
	}

	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("could not open package root: %w", err)
	}
	defer rootHandle.Close()

	file, err := rootHandle.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not access file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("could not access file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	return file, nil
}
