// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
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
// command lines discovered from live processes.
func findConfigPath(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader, findConfigArg func([]string) (string, bool)) (string, bool, error) {
	commandline, err := reader.ReadRuntimeCommandline(ctx)
	if err != nil {
		return "", false, err
	}
	if configArg, matched := findConfigArg(commandline.Args); matched {
		if configPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir); resolved {
			return configPath, true, nil
		}
	}

	var configPath string
	for _, commandline := range reader.ReadLiveProcessCommandlines(ctx) {
		configArg, matched := findConfigArg(commandline.Args)
		if !matched {
			continue
		}
		resolvedPath, resolved := resolveConfigPath(configArg, commandline.WorkingDir)
		if !resolved {
			return "", false, nil
		}
		if configPath != "" && configPath != resolvedPath {
			return "", false, nil
		}
		configPath = resolvedPath
	}
	return configPath, configPath != "", nil
}
