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
	"sort"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

const defaultExecutablePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

var managedEnvironmentVariables = map[string]struct{}{
	"HOME":   {},
	"PATH":   {},
	"TMPDIR": {},
}

// BuildEnvironment creates the restricted environment available to an authored script.
func BuildEnvironment(pkg *Package, session *Session) ([]string, error) {
	if pkg == nil || pkg.Manifest == nil {
		return nil, errors.New("authored-script package is required")
	}
	if session == nil {
		return nil, errors.New("authored-script session is required")
	}

	executablePaths := make([]string, 0, len(pkg.ToolDirectories)+1)
	for _, directory := range pkg.ToolDirectories {
		if strings.ContainsRune(directory, os.PathListSeparator) {
			return nil, fmt.Errorf("authored-script tool directory %q contains a path separator", directory)
		}
		executablePaths = append(executablePaths, directory)
	}
	executablePaths = append(executablePaths, defaultExecutablePath)

	environment := map[string]string{
		"HOME":   session.HomeDirectory,
		"PATH":   strings.Join(executablePaths, string(os.PathListSeparator)),
		"TMPDIR": session.TempDirectory,
	}
	declaredVariables := make(map[string]string,
		len(pkg.Manifest.Config.AllowedEnvVars)+len(pkg.Manifest.Config.SetGlobalEnvVars)+len(pkg.Manifest.Config.SetSessionEnvVars))
	for _, name := range pkg.Manifest.Config.AllowedEnvVars {
		if err := validateEnvironmentName(name); err != nil {
			return nil, err
		}
		if previousScope, found := declaredVariables[name]; found {
			return nil, fmt.Errorf("authored-script environment variable %q is declared more than once as %s", name, previousScope)
		}
		declaredVariables[name] = "allowed"
		if _, managed := managedEnvironmentVariables[name]; managed {
			continue
		}
		if value, found := os.LookupEnv(name); found {
			environment[name] = value
		}
	}

	if err := registerEnvironmentVariables("global", pkg.Manifest.Config.SetGlobalEnvVars, declaredVariables); err != nil {
		return nil, err
	}
	if err := registerEnvironmentVariables("session", pkg.Manifest.Config.SetSessionEnvVars, declaredVariables); err != nil {
		return nil, err
	}

	if len(pkg.Manifest.Config.SetGlobalEnvVars) > 0 {
		globalDirectory, err := globalEnvironmentDirectory(pkg.ArtifactDigest)
		if err != nil {
			return nil, err
		}
		for _, variable := range pkg.Manifest.Config.SetGlobalEnvVars {
			value, err := materializeEnvironmentVariable(globalDirectory, "global", variable)
			if err != nil {
				return nil, err
			}
			environment[variable.Name] = value
		}
	}
	for _, variable := range pkg.Manifest.Config.SetSessionEnvVars {
		value, err := materializeEnvironmentVariable(session.RootDirectory, "session", variable)
		if err != nil {
			return nil, err
		}
		environment[variable.Name] = value
	}

	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]string, 0, len(names))
	for _, name := range names {
		if strings.IndexByte(environment[name], 0) >= 0 {
			return nil, fmt.Errorf("authored-script environment variable %q contains a NUL byte", name)
		}
		result = append(result, name+"="+environment[name])
	}
	return result, nil
}

func registerEnvironmentVariables(scope string, variables []EnvironmentVariable, declaredVariables map[string]string) error {
	for _, variable := range variables {
		if err := validateEnvironmentName(variable.Name); err != nil {
			return err
		}
		if previousScope, found := declaredVariables[variable.Name]; found {
			if previousScope == scope {
				return fmt.Errorf("authored-script %s environment variable %q is declared more than once", scope, variable.Name)
			}
			return fmt.Errorf("authored-script environment variable %q cannot be declared as both %s and %s", variable.Name, previousScope, scope)
		}
		if _, managed := managedEnvironmentVariables[variable.Name]; managed {
			return fmt.Errorf("authored-script %s environment variable %q is managed by PAR", scope, variable.Name)
		}
		declaredVariables[variable.Name] = scope
	}
	return nil
}

func materializeEnvironmentVariable(root, scope string, variable EnvironmentVariable) (string, error) {
	if variable.Kind == environmentKindValue {
		return variable.Value, nil
	}
	if !filepath.IsLocal(variable.Value) {
		return "", fmt.Errorf("authored-script %s environment variable %q path %q is not relative to its state directory", scope, variable.Name, variable.Value)
	}

	resolvedPath, err := securejoin.SecureJoin(root, variable.Value)
	if err != nil {
		return "", fmt.Errorf("could not resolve authored-script %s environment variable %q: %w", scope, variable.Name, err)
	}
	if resolvedPath == root {
		return "", fmt.Errorf("authored-script %s environment variable %q cannot use its state root", scope, variable.Name)
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
	if err := os.MkdirAll(path, sessionDirectoryMode); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	return nil
}

func validateEnvironmentName(name string) error {
	if name == "" || !isEnvironmentNameStart(name[0]) {
		return fmt.Errorf("invalid authored-script environment variable name %q", name)
	}
	for i := 1; i < len(name); i++ {
		if !isEnvironmentNameStart(name[i]) && (name[i] < '0' || name[i] > '9') {
			return fmt.Errorf("invalid authored-script environment variable name %q", name)
		}
	}
	return nil
}

func isEnvironmentNameStart(character byte) bool {
	return character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}
