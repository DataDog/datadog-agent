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
	allowedVariableNames := make(map[string]struct{}, len(pkg.Manifest.Config.AllowedEnvVars))
	for _, name := range pkg.Manifest.Config.AllowedEnvVars {
		if err := validateEnvironmentName(name); err != nil {
			return nil, err
		}
		allowedVariableNames[name] = struct{}{}
		if _, managed := managedEnvironmentVariables[name]; managed {
			continue
		}
		if value, found := os.LookupEnv(name); found {
			environment[name] = value
		}
	}

	sessionVariableNames := make(map[string]struct{}, len(pkg.Manifest.Config.SetSessionEnvVars))
	for _, variable := range pkg.Manifest.Config.SetSessionEnvVars {
		if err := validateEnvironmentName(variable.Name); err != nil {
			return nil, err
		}
		if _, found := sessionVariableNames[variable.Name]; found {
			return nil, fmt.Errorf("authored-script session environment variable %q is declared more than once", variable.Name)
		}
		sessionVariableNames[variable.Name] = struct{}{}
		if _, found := allowedVariableNames[variable.Name]; found {
			return nil, fmt.Errorf("authored-script environment variable %q cannot be both allowed and set for the session", variable.Name)
		}
		if _, managed := managedEnvironmentVariables[variable.Name]; managed {
			return nil, fmt.Errorf("authored-script session environment variable %q is managed by PAR", variable.Name)
		}
	}

	for _, variable := range pkg.Manifest.Config.SetSessionEnvVars {
		value, err := materializeSessionEnvironmentVariable(session.RootDirectory, variable)
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

func materializeSessionEnvironmentVariable(root string, variable EnvironmentVariable) (string, error) {
	if variable.Kind == environmentKindValue {
		return variable.Value, nil
	}
	if !filepath.IsLocal(variable.Value) {
		return "", fmt.Errorf("authored-script session environment variable %q path %q is not relative to the session", variable.Name, variable.Value)
	}

	resolvedPath, err := securejoin.SecureJoin(root, variable.Value)
	if err != nil {
		return "", fmt.Errorf("could not resolve authored-script session environment variable %q: %w", variable.Name, err)
	}
	if resolvedPath == root {
		return "", fmt.Errorf("authored-script session environment variable %q cannot use the session root", variable.Name)
	}
	if err := os.MkdirAll(filepath.Dir(resolvedPath), sessionDirectoryMode); err != nil {
		return "", fmt.Errorf("could not create parent directory for authored-script session environment variable %q: %w", variable.Name, err)
	}

	switch variable.Kind {
	case environmentKindFile:
		if err := ensureSessionFile(resolvedPath); err != nil {
			return "", fmt.Errorf("could not prepare file for authored-script session environment variable %q: %w", variable.Name, err)
		}
	case environmentKindDirectory:
		if err := os.MkdirAll(resolvedPath, sessionDirectoryMode); err != nil {
			return "", fmt.Errorf("could not prepare directory for authored-script session environment variable %q: %w", variable.Name, err)
		}
	default:
		return "", fmt.Errorf("authored-script session environment variable %q has unsupported kind %q", variable.Name, variable.Kind)
	}
	return resolvedPath, nil
}

func ensureSessionFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		return file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
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
