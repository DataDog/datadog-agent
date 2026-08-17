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

// readConfigFile discovers and reads a command-line config file, then considers
// fallbackConfigArg before ordered groups of default paths. The first group with
// readable files wins, and exactly one file must be readable within that group.
// Returns the file and whether one was selected.
func readConfigFile(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	findConfigArg func([]string) (string, bool),
	matchesCommandline func([]string) bool,
	fallbackConfigArg string,
	defaultPathGroups ...[]string,
) (configfilesdiscoveryimpl.ConfigFile, bool, error) {
	// Runtime metadata has highest precedence and provides the working directory
	// needed to resolve a relative config argument or fallbackConfigArg.
	commandline, commandlineErr := reader.ReadRuntimeCommandline(ctx)
	runtimeWorkingDir := ""
	commandMatched := false
	configArgFound := false
	configPath := ""
	if commandlineErr == nil {
		runtimeWorkingDir = commandline.WorkingDir
		commandMatched = matchesCommandline(commandline.Args)
		if configArg, found := findConfigArg(commandline.Args); found {
			commandMatched = true
			configArgFound = true
			if resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir); resolved {
				configPath = resolvedPath
			}
		}
	}

	// If runtime metadata has no resolvable path, inspect live process command
	// lines. Every discovered path must resolve, and multiple paths must agree.
	if configPath == "" {
		for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
			if matchesCommandline(commandline.Args) {
				commandMatched = true
			}
			configArg, found := findConfigArg(commandline.Args)
			if !found {
				continue
			}
			configArgFound = true
			resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
			if !resolved {
				return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
			}
			if configPath != "" && configPath != resolvedPath {
				return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
			}
			configPath = resolvedPath
		}
	}

	// Only use the caller-provided fallback when argv did not explicitly name a
	// config file. An unresolved argv path is authoritative and blocks guessing.
	if configPath == "" && !configArgFound && fallbackConfigArg != "" {
		var resolved bool
		configPath, resolved = resolveConfigPath(fallbackConfigArg, runtimeWorkingDir)
		if !resolved {
			return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
		}
	}

	// Explicit paths are authoritative. A read failure must not fall through to
	// conventional defaults that the process might not have loaded.
	if configPath != "" {
		file, err := reader.ReadFile(ctx, configPath)
		if err != nil {
			return configfilesdiscoveryimpl.ConfigFile{}, false, fmt.Errorf("read explicit config file %q: %w", configPath, err)
		}
		return file, true, nil
	}

	// A matching command line or an explicit but unusable source also blocks
	// defaults, since selecting one would only be a guess.
	if configArgFound || fallbackConfigArg != "" || commandMatched {
		return configfilesdiscoveryimpl.ConfigFile{}, false, nil
	}

	// Default groups are ordered by priority. The first group with readable files
	// wins, but collection is suppressed when that group is ambiguous.
	for _, defaultPaths := range defaultPathGroups {
		files := make([]configfilesdiscoveryimpl.ConfigFile, 0, len(defaultPaths))
		for _, path := range defaultPaths {
			file, err := reader.ReadFile(ctx, path)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return configfilesdiscoveryimpl.ConfigFile{}, false, ctxErr
				}
				continue
			}
			files = append(files, file)
		}

		switch len(files) {
		case 0:
			continue
		case 1:
			return files[0], true, nil
		default:
			return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
		}
	}

	return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
}
