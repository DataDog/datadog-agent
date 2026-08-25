// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

const (
	defaultExecutablePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// managedEnvironmentVariables are set by BuildEnvironment from session/package state.
// AllowedEnvVars or setSessionEnvVars entries with these names will error out, since the
// script cannot pass through or override the runner's own value for them.
var managedEnvironmentVariables = map[string]struct{}{
	"HOME":   {},
	"PATH":   {},
	"TMPDIR": {},
}

// BuildEnvironment creates the environment available to an authored script.
func (pkg *Package) BuildEnvironment(session *Session) ([]string, error) {
	if pkg == nil || pkg.Manifest == nil {
		return nil, errors.New("authored-script package is required")
	}
	if session == nil {
		return nil, errors.New("authored-script session is required")
	}

	executablePath, err := buildExecutablePath(pkg.ToolPaths)
	if err != nil {
		return nil, err
	}
	environment := map[string]string{
		"HOME":   session.HomeDirectory,
		"PATH":   executablePath,
		"TMPDIR": session.TempDirectory,
	}
	for _, name := range pkg.Manifest.Config.AllowedEnvVars {
		if _, managed := managedEnvironmentVariables[name]; managed {
			return nil, fmt.Errorf("authored-script environment variable %q is managed by PAR and cannot be declared as an allowed environment variable", name)
		}
		if value, found := os.LookupEnv(name); found {
			environment[name] = value
		}
	}

	for _, variable := range pkg.Manifest.Config.SetSessionEnvVars {
		if _, managed := managedEnvironmentVariables[variable.Name]; managed {
			return nil, fmt.Errorf("authored-script session environment variable %q cannot override the managed %q value", variable.Name, variable.Name)
		}
		value, err := materializeEnvironmentVariable(session.RootDirectory, "session", variable)
		if err != nil {
			return nil, err
		}
		environment[variable.Name] = value
	}

	result := make([]string, 0, len(environment))
	for name, value := range environment {
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("authored-script environment variable %q contains a NUL byte", name)
		}
		result = append(result, name+"="+value)
	}
	return result, nil
}

func buildExecutablePath(toolPaths []string) (string, error) {
	seenDirectories := make(map[string]struct{}, len(toolPaths)+1)
	executablePaths := make([]string, 0, len(toolPaths)+1)
	for _, toolPath := range toolPaths {
		directory := filepath.Dir(toolPath)
		if strings.ContainsRune(directory, os.PathListSeparator) {
			return "", fmt.Errorf("authored-script tool directory %q contains a path separator", directory)
		}
		if _, seen := seenDirectories[directory]; seen {
			continue
		}
		seenDirectories[directory] = struct{}{}
		executablePaths = append(executablePaths, directory)
	}
	executablePaths = append(executablePaths, defaultExecutablePath)
	return strings.Join(executablePaths, string(os.PathListSeparator)), nil
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
