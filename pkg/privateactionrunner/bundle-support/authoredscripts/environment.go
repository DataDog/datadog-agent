// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// BuildEnvironment creates the environment available to an authored script.
func (pkg *Package) BuildEnvironment(session *Session, parameters map[string]interface{}) ([]string, error) {
	if pkg == nil || pkg.Manifest == nil {
		return nil, errors.New("authored-script package is required")
	}
	if session == nil {
		return nil, errors.New("authored-script session is required")
	}

	envVars, err := platformEnvironment(session, pkg.ExecutableDirectories)
	if err != nil {
		return nil, err
	}
	// Platform-provided variables are managed by PAR and cannot be overridden by package metadata.
	managedVariables := make(map[string]struct{}, len(envVars))
	for name := range envVars {
		managedVariables[normalizeEnvironmentVariableName(name)] = struct{}{}
	}
	for _, name := range pkg.Manifest.AllowedEnvVars {
		if _, managed := managedVariables[normalizeEnvironmentVariableName(name)]; managed {
			return nil, fmt.Errorf("authored-script environment variable %q is managed by PAR and cannot be declared as an allowed environment variable", name)
		}
		if value, found := os.LookupEnv(name); found {
			setEnvironmentEntry(envVars, name, value)
		}
	}

	for _, variable := range pkg.Manifest.SetSessionEnvVars {
		if _, managed := managedVariables[normalizeEnvironmentVariableName(variable.Name)]; managed {
			return nil, fmt.Errorf("authored-script session environment variable %q cannot override the managed %q value", variable.Name, variable.Name)
		}
		value, err := materializeEnvironmentVariable(session.RootDirectory, "session", variable)
		if err != nil {
			return nil, err
		}
		setEnvironmentEntry(envVars, variable.Name, value)
	}

	if err := addParameterEnvironment(envVars, pkg.Manifest.ParameterEnvMapping, parameters); err != nil {
		return nil, err
	}

	result := make([]string, 0, len(envVars))
	for name, value := range envVars {
		if err := validateEnvironmentVariableName(name); err != nil {
			return nil, err
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("authored-script environment variable %q contains a NUL byte", name)
		}
		result = append(result, name+"="+value)
	}
	return result, nil
}

// addParameterEnvironment converts input parameters into environment variable
// assignments in the environment.
func addParameterEnvironment(envVars map[string]string, parameterEnvMapping map[string]string, parameters map[string]interface{}) error {
	for name, value := range parameters {
		envName, ok := parameterEnvMapping[name]
		if !ok {
			return fmt.Errorf("authored-script parameter %q has no configured environment variable mapping", name)
		}
		if _, exists := envVars[normalizeEnvironmentVariableName(envName)]; exists {
			return fmt.Errorf("authored-script parameter %q maps to environment variable %q, which is already set", name, envName)
		}

		var stringValue string
		if s, ok := value.(string); ok {
			stringValue = s
		} else {
			encoded, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to encode authored-script parameter %q: %w", name, err)
			}
			stringValue = string(encoded)
		}
		setEnvironmentEntry(envVars, envName, stringValue)
	}
	return nil
}

func buildExecutablePath(executableDirectories []string) (string, error) {
	defaultPath, err := platformDefaultExecutablePath()
	if err != nil {
		return "", err
	}

	seenDirectories := make(map[string]struct{}, len(executableDirectories)+1)
	executablePaths := make([]string, 0, len(executableDirectories)+1)
	for _, directory := range executableDirectories {
		if strings.ContainsRune(directory, os.PathListSeparator) {
			return "", fmt.Errorf("authored-script tool directory %q contains a path separator", directory)
		}
		if _, seen := seenDirectories[directory]; seen {
			continue
		}
		seenDirectories[directory] = struct{}{}
		executablePaths = append(executablePaths, directory)
	}
	executablePaths = append(executablePaths, defaultPath)
	return strings.Join(executablePaths, string(os.PathListSeparator)), nil
}

func setEnvironmentEntry(envVars map[string]string, name, value string) {
	envVars[normalizeEnvironmentVariableName(name)] = value
}

func validateEnvironmentVariableName(name string) error {
	if name == "" {
		return errors.New("authored-script environment variable name cannot be empty")
	}
	if strings.IndexByte(name, '=') >= 0 {
		return fmt.Errorf("authored-script environment variable name %q contains an equals sign", name)
	}
	if strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("authored-script environment variable name %q contains a NUL byte", name)
	}
	return nil
}

func materializeEnvironmentVariable(root, scope string, variable EnvironmentVariable) (string, error) {
	if variable.Kind == environmentKindValue {
		return variable.Value, nil
	}
	if !filepath.IsLocal(variable.Value) {
		return "", fmt.Errorf("authored-script %s environment variable %q path %q is not relative to its session directory", scope, variable.Name, variable.Value)
	}

	resolvedPath, err := securejoin.SecureJoin(root, variable.Value)
	if err != nil {
		return "", fmt.Errorf("could not resolve authored-script %s environment variable %q: %w", scope, variable.Name, err)
	}
	if resolvedPath == root {
		return "", fmt.Errorf("authored-script %s environment variable %q cannot use its session directory root", scope, variable.Name)
	}
	if err := os.MkdirAll(filepath.Dir(resolvedPath), sessionDirectoryMode); err != nil {
		return "", fmt.Errorf("could not create parent directory for authored-script %s environment variable %q: %w", scope, variable.Name, err)
	}

	switch variable.Kind {
	case environmentKindFile:
		if err := ensureEnvironmentFile(resolvedPath); err != nil {
			return "", fmt.Errorf("could not prepare file for authored-script %s environment variable %q: %w", scope, variable.Name, err)
		}
	case environmentKindDirectory:
		if err := ensureEnvironmentDirectory(resolvedPath); err != nil {
			return "", fmt.Errorf("could not prepare directory for authored-script %s environment variable %q: %w", scope, variable.Name, err)
		}
	default:
		return "", fmt.Errorf("authored-script %s environment variable %q has unsupported kind %q", scope, variable.Name, variable.Kind)
	}
	return resolvedPath, nil
}

func ensureEnvironmentFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		return file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}
	return nil
}

func ensureEnvironmentDirectory(path string) error {
	return os.MkdirAll(path, sessionDirectoryMode)
}
