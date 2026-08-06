// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"
	"strings"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
)

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

// findConfigPath tries the runtime-native command first, then falls back to
// command lines discovered from live processes. Returns the resolved path and
// whether a command line for the service was found.
func findConfigPath(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	findConfigArg func([]string) (string, bool),
	matchesCommandline func([]string) bool,
) (string, bool, error) {
	commandline, runtimeErr := reader.ReadRuntimeCommandline(ctx)
	commandMatched := false
	if runtimeErr == nil {
		commandMatched = matchesCommandline(commandline.Args)
		if configArg, found := findConfigArg(commandline.Args); found {
			commandMatched = true
			if configPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir); resolved {
				return configPath, true, nil
			}
		}
	}

	var configPath string
	liveCommandMatched := false
	for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
		if matchesCommandline(commandline.Args) {
			commandMatched = true
			liveCommandMatched = true
		}
		configArg, found := findConfigArg(commandline.Args)
		if !found {
			continue
		}
		commandMatched = true
		resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
		if !resolved {
			return "", true, runtimeErr
		}
		if configPath != "" && configPath != resolvedPath {
			return "", true, runtimeErr
		}
		configPath = resolvedPath
	}
	if configPath != "" {
		return configPath, true, nil
	}
	if liveCommandMatched {
		return "", true, nil
	}
	return "", commandMatched, runtimeErr
}

// readConfigFile discovers and reads an explicit config file, or falls back to
// default paths when no command line for the service is found. Returns the file
// and whether exactly one file was selected.
func readConfigFile(
	ctx context.Context,
	reader configfilesdiscoveryimpl.ConfigReader,
	findConfigArg func([]string) (string, bool),
	matchesCommandline func([]string) bool,
	defaultPaths []string,
) (configfilesdiscoveryimpl.ConfigFile, bool, error) {
	configPath, commandMatched, commandlineErr := findConfigPath(ctx, reader, findConfigArg, matchesCommandline)
	if configPath != "" {
		file, err := reader.ReadFile(ctx, configPath)
		if err != nil {
			return configfilesdiscoveryimpl.ConfigFile{}, false, fmt.Errorf("read explicit config file %q: %w", configPath, err)
		}
		return file, true, nil
	}
	if commandMatched {
		return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
	}

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
		return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
	case 1:
		return files[0], true, nil
	default:
		return configfilesdiscoveryimpl.ConfigFile{}, false, commandlineErr
	}
}
