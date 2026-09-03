// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
)

// readEnvVars returns selected environment variables in deterministic name order.
func readEnvVars(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate,
) ([]configfilesdiscoveryimpl.ConfigEnvVar, error) {
	env, err := reader.ReadEnvVars(ctx, predicate)
	if err != nil {
		return nil, err
	}
	if len(env) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	envVars := make([]configfilesdiscoveryimpl.ConfigEnvVar, 0, len(env))
	for _, name := range names {
		envVars = append(envVars, configfilesdiscoveryimpl.ConfigEnvVar{Name: name, Value: env[name]})
	}
	return envVars, nil
}

func unwrapShellCommandline(args []string) []string {
	if len(args) < 3 || !isShellExecutable(args[0]) || args[1] != "-c" {
		return args
	}
	return strings.Fields(args[2])
}

func isShellExecutable(arg string) bool {
	switch path.Base(arg) {
	case "sh", "bash", "dash", "ash", "zsh":
		return true
	default:
		return false
	}
}

func resolveConfigPath(configPath string, workingDir string) (string, bool) {
	if configPath == "" || strings.ContainsRune(configPath, 0) {
		return "", false
	}
	if path.IsAbs(configPath) {
		return path.Clean(configPath), true
	}
	if !path.IsAbs(workingDir) {
		return "", false
	}
	return path.Clean(path.Join(workingDir, configPath)), true
}

// cleanWorkingDir returns a cleaned absolute working directory, or an empty
// string when workingDir is not absolute.
func cleanWorkingDir(workingDir string) string {
	if !path.IsAbs(workingDir) {
		return ""
	}
	return path.Clean(workingDir)
}

// configFileSelection describes the config file selected for collection and the
// reliable process working directory associated with it, when available.
type configFileSelection struct {
	file       configfilesdiscoveryimpl.ConfigFile
	path       configfilesdiscoveryimpl.VerifiedConfigFilePath
	workingDir string
}

// selectConfigFile discovers and reads a command-line config file, then considers
// fallbackConfigArg before ordered groups of default paths. The first group with
// readable files wins, and exactly one file must be readable within that group.
// Returns the selected file and its process working directory when available.
// A nil selection means no file was found.
func selectConfigFile(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	findConfigArg func([]string) (string, bool),
	matchesCommandline func([]string) bool,
	fallbackConfigArg string,
	defaultPathGroups ...[]string,
) (*configFileSelection, error) {
	// Runtime metadata has highest precedence and provides the working directory
	// needed to resolve a relative config argument or fallbackConfigArg.
	commandline, commandlineErr := reader.ReadRuntimeCommandline(ctx)
	runtimeWorkingDir := ""
	if commandlineErr == nil {
		runtimeWorkingDir = cleanWorkingDir(commandline.WorkingDir)
		if configArg, found := findConfigArg(commandline.Args); found {
			if resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir); resolved {
				verifiedPath, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(resolvedPath))
				if err != nil {
					return nil, fmt.Errorf("read explicit config file %q: %w", resolvedPath, err)
				}
				file, err := reader.ReadFile(ctx, verifiedPath)
				if err != nil {
					return nil, fmt.Errorf("read explicit config file %q: %w", resolvedPath, err)
				}
				return &configFileSelection{file: file, path: verifiedPath, workingDir: runtimeWorkingDir}, nil
			}
		}
	}

	// If runtime metadata has no resolvable path, inspect live process command
	// lines. Use the first process that explicitly identifies a resolvable config
	// file and keep the CWD from that same process.
	liveCommandlines := reader.ReadLiveProcessCommandlines(ctx)
	for _, commandline := range liveCommandlines {
		configArg, found := findConfigArg(commandline.Args)
		if !found {
			continue
		}
		resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
		if !resolved {
			return nil, commandlineErr
		}
		verifiedPath, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(resolvedPath))
		if err != nil {
			return nil, fmt.Errorf("read explicit config file %q: %w", resolvedPath, err)
		}
		file, err := reader.ReadFile(ctx, verifiedPath)
		if err != nil {
			return nil, fmt.Errorf("read explicit config file %q: %w", resolvedPath, err)
		}
		return &configFileSelection{file: file, path: verifiedPath, workingDir: cleanWorkingDir(commandline.WorkingDir)}, nil
	}

	// Only use the caller-provided fallback when argv did not explicitly name a
	// config file. An unresolved argv path is authoritative and blocks guessing.
	if commandlineErr == nil {
		if _, found := findConfigArg(commandline.Args); found {
			return nil, nil
		}
	}
	if fallbackConfigArg != "" {
		configPath, resolved := resolveConfigPath(fallbackConfigArg, runtimeWorkingDir)
		if !resolved {
			return nil, commandlineErr
		}
		verifiedPath, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(configPath))
		if err != nil {
			return nil, fmt.Errorf("read explicit config file %q: %w", configPath, err)
		}
		file, err := reader.ReadFile(ctx, verifiedPath)
		if err != nil {
			return nil, fmt.Errorf("read explicit config file %q: %w", configPath, err)
		}
		return &configFileSelection{file: file, path: verifiedPath, workingDir: runtimeWorkingDir}, nil
	}

	// A matching command line or an explicit but unusable source also blocks
	// defaults, since selecting one would only be a guess.
	if commandlineErr == nil && matchesCommandline(commandline.Args) {
		return nil, nil
	}
	for _, commandline := range liveCommandlines {
		if matchesCommandline(commandline.Args) {
			return nil, nil
		}
	}

	// Default groups are ordered by priority. The first group with readable files
	// wins, but collection is suppressed when that group is ambiguous.
	for _, defaultPaths := range defaultPathGroups {
		selections := make([]configFileSelection, 0, len(defaultPaths))
		for _, path := range defaultPaths {
			verifiedPath, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(path))
			if err != nil {
				continue
			}
			file, err := reader.ReadFile(ctx, verifiedPath)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, ctxErr
				}
				continue
			}
			selections = append(selections, configFileSelection{file: file, path: verifiedPath})
		}

		switch len(selections) {
		case 0:
			continue
		case 1:
			return &selections[0], nil
		default:
			return nil, commandlineErr
		}
	}

	return nil, commandlineErr
}
